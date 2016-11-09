package pipeline

import (
	"bytes"
	"errors"
	"io/ioutil"
	"sync"

	log "github.com/Sirupsen/logrus"

	"golang.org/x/net/context"

	"github.com/go-yaml/yaml"
	"github.com/walter-cd/walter/lib/notify"
	"github.com/walter-cd/walter/lib/task"
)

type Pipeline struct {
	Build     Build
	Deploy    Deploy
	Notifiers []notify.Notifier
}

type Build struct {
	Tasks   Tasks
	Cleanup Tasks
}

type Deploy struct {
	Tasks   Tasks
	Cleanup Tasks
}

type Tasks []*task.Task

func Load(b []byte) (Pipeline, error) {
	p := Pipeline{}
	err := yaml.Unmarshal(b, &p)
	if err == nil {
		p.Notifiers, err = notify.NewNotifiers(b)
		return p, err
	}

	t := Tasks{}
	err = yaml.Unmarshal(b, &t)
	if err != nil {
		log.Error(err)
	}

	p.Build.Tasks = t
	return p, nil
}

func LoadFromFile(file string) (Pipeline, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return Pipeline{}, err
	}
	return Load(data)
}

func (p *Pipeline) Run() int {
	failed := false

	log.Info("Build started")
	ctx, cancel := context.WithCancel(context.Background())
	err := p.runTasks(ctx, cancel, p.Build.Tasks, nil)
	if err != nil {
		log.Error(err)
		log.Error("Build failed")
		failed = true
	} else {
		log.Info("Build succeeded")
	}

	log.Info("Build cleanup started")
	ctx, cancel = context.WithCancel(context.Background())
	err = p.runTasks(ctx, cancel, p.Build.Cleanup, nil)
	if err != nil {
		log.Error(err)
		log.Error("Build cleanup failed")
		failed = true
	} else {
		log.Info("Build cleanup failed")
	}

	if failed {
		return 1
	}

	log.Info("Deploy started")
	ctx, cancel = context.WithCancel(context.Background())
	err = p.runTasks(ctx, cancel, p.Deploy.Tasks, nil)
	if err != nil {
		log.Error(err)
		log.Error("Deploy failed")
		failed = true
	} else {
		log.Info("Deploy succeeded")
	}

	ctx, cancel = context.WithCancel(context.Background())
	err = p.runTasks(ctx, cancel, p.Deploy.Cleanup, nil)
	if err != nil {
		log.Error(err)
		log.Error("Deploy cleanup failed")
		failed = true
	} else {
		log.Info("Deploy cleanup succeeded")
	}

	if failed {
		return 1
	}

	return 0
}

func includeTasks(file string) (Tasks, error) {
	data, err := ioutil.ReadFile(file)
	tasks := Tasks{}
	if err != nil {
		return tasks, err
	}

	err = yaml.Unmarshal(data, &tasks)
	if err != nil {
		return tasks, err
	}

	return tasks, err
}

func (p *Pipeline) runTasks(ctx context.Context, cancel context.CancelFunc, tasks Tasks, prevTask *task.Task) error {
	failed := false
	for i, t := range tasks {
		if i > 0 {
			prevTask = tasks[i-1]
		}

		if t.Include != "" {
			include, err := includeTasks(t.Include)
			if err != nil {
				log.Error(err)
				return err
			}
			p.runTasks(ctx, cancel, include, prevTask)
			continue
		}

		if len(t.Parallel) > 0 {
			p.runParallel(ctx, cancel, t, prevTask)
			continue
		}

		if len(t.Serial) > 0 {
			p.runSerial(ctx, cancel, t, prevTask)
			continue
		}

		if failed || (i > 0 && tasks[i-1].Status == task.Failed) {
			t.Status = task.Skipped
			failed = true
			log.Warnf("[%s] Task skipped because previous task failed", t.Name)
			continue
		}

		err := t.Run(ctx, cancel, prevTask)
		if err != nil {
			log.Errorf("[%s] %s", t.Name, err)
		}

		for _, n := range p.Notifiers {
			n.Notify(t)
		}
	}

	if failed {
		return errors.New("One of the tasks failed")
	}

	return nil
}

func (p *Pipeline) runParallel(ctx context.Context, cancel context.CancelFunc, t *task.Task, prevTask *task.Task) {

	var tasks Tasks
	for _, child := range t.Parallel {
		if child.Include != "" {
			include, err := includeTasks(child.Include)
			if err != nil {
				log.Error(err)
			}
			tasks = append(tasks, include...)
		} else {
			tasks = append(tasks, child)
		}
	}

	log.Infof("[%s] Start task", t.Name)

	var wg sync.WaitGroup
	for _, t := range tasks {
		wg.Add(1)
		go func(t *task.Task) {
			defer wg.Done()

			if len(t.Serial) > 0 {
				p.runSerial(ctx, cancel, t, prevTask)
				return
			}

			t.Run(ctx, cancel, prevTask)

			for _, n := range p.Notifiers {
				n.Notify(t)
			}
		}(t)
	}
	wg.Wait()

	t.Status = task.Succeeded

	t.Stdout = new(bytes.Buffer)
	t.Stderr = new(bytes.Buffer)
	t.CombinedOutput = new(bytes.Buffer)

	for _, child := range tasks {
		t.Stdout.Write(child.Stdout.Bytes())
		t.Stderr.Write(child.Stderr.Bytes())
		t.CombinedOutput.Write(child.CombinedOutput.Bytes())
		if child.Status == task.Failed {
			t.Status = task.Failed
		}
	}

	log.Infof("[%s] End task", t.Name)
}

func (p *Pipeline) runSerial(ctx context.Context, cancel context.CancelFunc, t *task.Task, prevTask *task.Task) {
	var tasks Tasks
	for _, child := range t.Serial {
		if child.Include != "" {
			include, err := includeTasks(child.Include)
			if err != nil {
				log.Error(err)
			}
			tasks = append(tasks, include...)
		} else {
			tasks = append(tasks, child)
		}
	}

	log.Infof("[%s] Start task", t.Name)

	p.runTasks(ctx, cancel, tasks, prevTask)
	t.Status = task.Succeeded
	for _, child := range tasks {
		if child.Status == task.Failed {
			t.Status = task.Failed
		}
	}

	t.Stdout = new(bytes.Buffer)
	t.Stderr = new(bytes.Buffer)
	t.CombinedOutput = new(bytes.Buffer)

	lastTask := tasks[len(tasks)-1]
	t.Stdout.Write(lastTask.Stdout.Bytes())
	t.Stderr.Write(lastTask.Stderr.Bytes())
	t.CombinedOutput.Write(lastTask.CombinedOutput.Bytes())

	log.Infof("[%s] End task", t.Name)
}
