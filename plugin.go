package gendoc

import (
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/golang/protobuf/proto"
	plugin_go "github.com/golang/protobuf/protoc-gen-go/plugin"
	"github.com/pseudomuto/protokit"
)

// PluginOptions encapsulates options for the plugin. The type of renderer, template file, and the name of the output
// file are included.
type PluginOptions struct {
	Type                RenderType
	TemplateFile         string
	OutputFile           string
	ExcludePatterns      []*regexp.Regexp
	SourceRelative       bool
	CamelCaseFields      bool
	ExcludeDirectives    []string // Directives for paragraph/block exclusion (default: ["@exclude"])
	ExcludeLineDirectives []string // Directives for line-level exclusion (default: ["@exclude-line"])
}

// SupportedFeatures describes a flag setting for supported features.
var SupportedFeatures = uint64(plugin_go.CodeGeneratorResponse_FEATURE_SUPPORTS_EDITIONS)

// Plugin describes a protoc code generate plugin. It's an implementation of Plugin from github.com/pseudomuto/protokit
type Plugin struct{}

// Generate compiles the documentation and generates the CodeGeneratorResponse to send back to protoc. It does this
// by rendering a template based on the options parsed from the CodeGeneratorRequest.
func (p *Plugin) Generate(r *plugin_go.CodeGeneratorRequest) (*plugin_go.CodeGeneratorResponse, error) {
	options, err := ParseOptions(r)
	if err != nil {
		return nil, err
	}

	result := excludeUnwantedProtos(protokit.ParseCodeGenRequest(r), options.ExcludePatterns)

	customTemplate := ""

	if options.TemplateFile != "" {
		data, err := ioutil.ReadFile(options.TemplateFile)
		if err != nil {
			return nil, err
		}

		customTemplate = string(data)
	}

	resp := new(plugin_go.CodeGeneratorResponse)
	fdsGroup := groupProtosByDirectory(result, options.SourceRelative)
	for dir, fds := range fdsGroup {
		template := NewTemplate(fds, options)

		output, err := RenderTemplate(options.Type, template, customTemplate)
		if err != nil {
			return nil, err
		}

		resp.File = append(resp.File, &plugin_go.CodeGeneratorResponse_File{
			Name:    proto.String(filepath.Join(dir, options.OutputFile)),
			Content: proto.String(string(output)),
		})
	}

	resp.SupportedFeatures = proto.Uint64(SupportedFeatures)
	resp.MinimumEdition = proto.Int32(900)  // Edition_EDITION_LEGACY
	resp.MaximumEdition = proto.Int32(1001) // Edition_EDITION_2024

	return resp, nil
}

func groupProtosByDirectory(fds []*protokit.FileDescriptor, sourceRelative bool) map[string][]*protokit.FileDescriptor {
	fdsGroup := make(map[string][]*protokit.FileDescriptor)

	for _, fd := range fds {
		dir := ""
		if sourceRelative {
			dir, _ = filepath.Split(fd.GetName())
		}
		if dir == "" {
			dir = "./"
		}
		fdsGroup[dir] = append(fdsGroup[dir], fd)
	}
	return fdsGroup
}

func excludeUnwantedProtos(fds []*protokit.FileDescriptor, excludePatterns []*regexp.Regexp) []*protokit.FileDescriptor {
	descs := make([]*protokit.FileDescriptor, 0)

OUTER:
	for _, d := range fds {
		for _, p := range excludePatterns {
			if p.MatchString(d.GetName()) {
				continue OUTER
			}
		}

		descs = append(descs, d)
	}

	return descs
}

// ParseOptions parses plugin options from a CodeGeneratorRequest. It does this by splitting the `Parameter` field from
// the request object and parsing out the type of renderer to use and the name of the file to be generated.
//
// The parameter (`--doc_opt`) must be of the format <TYPE|TEMPLATE_FILE>,<OUTPUT_FILE>[,default|source_relative]:<OPTION>,<OPTION>*.
// The file will be written to the directory specified with the `--doc_out` argument to protoc.
func ParseOptions(req *plugin_go.CodeGeneratorRequest) (*PluginOptions, error) {
	options := &PluginOptions{
		Type:                RenderTypeHTML,
		TemplateFile:         "",
		OutputFile:           "index.html",
		SourceRelative:       false,
		CamelCaseFields:      false,
		ExcludeDirectives:    []string{"@exclude"},
		ExcludeLineDirectives: []string{"@exclude-line"},
	}

	params := strings.Split(req.GetParameter(), "\n")[0]
	colonParts := strings.SplitN(params, ":", 2)
	fileParams := colonParts[0]
	if len(colonParts) == 2 {
		optionsPart := colonParts[1]
		currentOption := ""
		for _, token := range strings.Split(optionsPart, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if strings.Contains(token, "=") {
				keyValue := strings.SplitN(token, "=", 2)
				key := keyValue[0]
				value := keyValue[1]
				currentOption = key
				switch key {
				case "camel_case_fields":
					switch value {
					case "true":
						options.CamelCaseFields = true
					case "false":
						options.CamelCaseFields = false
					default:
						return nil, fmt.Errorf("Invalid camel_case_fields value: %v", value)
					}
				case "exclude_patterns":
					if value != "" {
						r, err := regexp.Compile(value)
						if err != nil {
							return nil, err
						}
						options.ExcludePatterns = append(options.ExcludePatterns, r)
					}
				case "exclude_directive":
					if value != "" {
						options.ExcludeDirectives = append(options.ExcludeDirectives, value)
					}
				case "exclude_line_directive":
					if value != "" {
						options.ExcludeLineDirectives = append(options.ExcludeLineDirectives, value)
					}
				default:
					return nil, fmt.Errorf("Invalid option: %v", key)
				}
				continue
			}
			if currentOption == "exclude_patterns" {
				r, err := regexp.Compile(token)
				if err != nil {
					return nil, err
				}
				options.ExcludePatterns = append(options.ExcludePatterns, r)
				continue
			}
			return nil, fmt.Errorf("Invalid option: %v", token)
		}
	}
	if fileParams == "" {
		return options, nil
	}

	if !strings.Contains(fileParams, ",") {
		return nil, fmt.Errorf("Invalid parameter: %s", fileParams)
	}

	parts := strings.Split(fileParams, ",")
	if len(parts) < 2 || len(parts) > 3 {
		return nil, fmt.Errorf("Invalid parameter: %s", fileParams)
	}

	options.TemplateFile = parts[0]
	options.OutputFile = path.Base(parts[1])
	if len(parts) > 2 {
		switch parts[2] {
		case "source_relative":
			options.SourceRelative = true
		case "default":
			options.SourceRelative = false
		default:
			return nil, fmt.Errorf("Invalid parameter: %s", fileParams)
		}
	}
	options.SourceRelative = len(parts) > 2 && parts[2] == "source_relative"

	renderType, err := NewRenderType(options.TemplateFile)
	if err == nil {
		options.Type = renderType
		options.TemplateFile = ""
	}

	return options, nil
}
