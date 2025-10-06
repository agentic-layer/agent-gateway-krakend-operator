/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

// krakendConfigTemplate is the base KrakenD configuration template
const krakendConfigTemplate = `{
    "$schema": "https://www.krakend.io/schema/v2.10/krakend.json",
    "version": 3,
	"plugin": {
		"pattern": ".so",
		"folder": "/plugins/"
	},
    "port": {{.Port}},
    "extra_config": {
		"plugin/http-server": {
			"@comment_name": "Name order defines handler order. Last entry is outermost/first handler.",
			"name": [{{range $i, $pluginName := .PluginNames}}{{if $i}},{{end}}
				"{{$pluginName}}"{{end}}
			]
		},
        "router": {
            "disable_access_log": false,
            "hide_version_header": true
        }
    },
    "timeout": "{{.Timeout}}",
    "output_encoding": "json",
    "name": "agent-gateway-krakend",
    "endpoints": [{{range $i, $endpoint := .Endpoints}}{{if $i}},{{end}}
        {
            "endpoint": "{{$endpoint.Endpoint}}",
            "output_encoding": "{{$endpoint.OutputEncoding}}",
            "method": "{{$endpoint.Method}}",
            "backend": [{{range $j, $backend := $endpoint.Backend}}{{if $j}},{{end}}
                {
                    "host": [{{range $k, $host := $backend.Host}}{{if $k}},{{end}}"{{$host}}"{{end}}],
                    "url_pattern": "{{$backend.URLPattern}}"
                }{{end}}]
        }{{end}}]
}`
