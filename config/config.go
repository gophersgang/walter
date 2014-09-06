/* plumber: a deployment pipeline template
 * Copyright (C) 2014 Recruit Technologies Co., Ltd. and contributors
 * (see CONTRIBUTORS.md)
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package config

import (
	"flag"
	"io/ioutil"

	"github.com/go-yaml/yaml"
	"github.com/recruit-tech/plumber/log"
)

type Opts struct {
	PipelineFilePath string
}

func LoadOpts(arguments []string) *Opts {
	var pipelineFilePath string
	flag.StringVar(&pipelineFilePath, "c", "./pipeline.yml", "pipeline.yml file")
	flag.Parse()

	return &Opts{
		PipelineFilePath: pipelineFilePath,
	}
}

func ReadConfig(configFilePath string) *map[interface{}]interface{} {
	configData := make(map[interface{}]interface{})
	data, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Errorf("error :%v \n", err)
	}

	err = yaml.Unmarshal([]byte(data), &configData)
	if err != nil {
		log.Errorf("error :%v \n", err)
	}
	return &configData
}
