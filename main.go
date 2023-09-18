package main

import (
	_ "embed"
	"flag"
	"fmt"
	"strings"

	"go.uber.org/mock/mockgen/model"
	"google.golang.org/protobuf/compiler/protogen"
	"google.golang.org/protobuf/types/pluginpb"
)

type methodType int

const (
	methodTypeUnary methodType = iota
	methodTypeClientStream
	methodTypeServerStream
	methodTypeBidirectionalStream
)

func getMethodType(m *protogen.Method) methodType {
	if !m.Desc.IsStreamingClient() && !m.Desc.IsStreamingServer() {
		return methodTypeUnary
	}
	if !m.Desc.IsStreamingServer() {
		return methodTypeClientStream
	}
	if !m.Desc.IsStreamingClient() {
		return methodTypeServerStream
	}
	return methodTypeBidirectionalStream
}

func fileToModel(file *protogen.File) *model.Package {
	pkg := &model.Package{
		Name:    string(file.GoPackageName),
		PkgPath: string(file.GoImportPath),
	}

	for _, s := range file.Services {
		clientIface := &model.Interface{Name: fmt.Sprintf("%sClient", s.GoName)}
		serverIface := &model.Interface{Name: fmt.Sprintf("%sServer", s.GoName)}
		for _, m := range s.Methods {
			switch getMethodType(m) {
			case methodTypeUnary:
				clientMethod, serverMethod := makeUnaryMethods(m)
				clientIface.AddMethod(clientMethod)
				serverIface.AddMethod(serverMethod)
			case methodTypeServerStream:
				clientMethod, serverMethod, ifaces := makeServerStreamMethods(m)
				pkg.Interfaces = append(pkg.Interfaces, ifaces...)
				clientIface.AddMethod(clientMethod)
				serverIface.AddMethod(serverMethod)
			case methodTypeClientStream:
				clientMethod, serverMethod, ifaces := makeClientStreamMethods(m)
				pkg.Interfaces = append(pkg.Interfaces, ifaces...)
				clientIface.AddMethod(clientMethod)
				serverIface.AddMethod(serverMethod)
			case methodTypeBidirectionalStream:
				clientMethod, serverMethod, ifaces := makeBidirectionalStreamMethods(m)
				pkg.Interfaces = append(pkg.Interfaces, ifaces...)
				clientIface.AddMethod(clientMethod)
				serverIface.AddMethod(serverMethod)
			}
		}
		pkg.Interfaces = append(pkg.Interfaces, clientIface, serverIface)
	}

	return pkg
}

func makeUnaryMethods(m *protogen.Method) (*model.Method, *model.Method) {
	clientMethod := &model.Method{
		Name: m.GoName,
		In: []*model.Parameter{
			{Name: "ctx", Type: &model.NamedType{Package: "context", Type: "Context"}},
			{Name: "in", Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Input.GoIdent.GoImportPath), Type: m.Input.GoIdent.GoName}}},
		},
		Out: []*model.Parameter{
			{Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Output.GoIdent.GoImportPath), Type: m.Output.GoIdent.GoName}}},
			{Type: model.PredeclaredType("error")},
		},
		Variadic: &model.Parameter{Name: "opts", Type: &model.NamedType{Package: "google.golang.org/grpc", Type: "CallOption"}},
	}
	serverMethod := &model.Method{
		Name: m.GoName,
		In: []*model.Parameter{
			{Name: "ctx", Type: &model.NamedType{Package: "context", Type: "Context"}},
			{Name: "in", Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Input.GoIdent.GoImportPath), Type: m.Input.GoIdent.GoName}}},
		},
		Out: []*model.Parameter{
			{Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Output.GoIdent.GoImportPath), Type: m.Output.GoIdent.GoName}}},
			{Type: model.PredeclaredType("error")},
		},
	}
	return clientMethod, serverMethod
}

func makeServerStreamMethods(m *protogen.Method) (*model.Method, *model.Method, []*model.Interface) {
	clientIfaceName := fmt.Sprintf("%s_%sClient", m.Parent.GoName, m.GoName)
	serverIfaceName := fmt.Sprintf("%s_%sServer", m.Parent.GoName, m.GoName)
	clientMethod := &model.Method{
		Name: m.GoName,
		In: []*model.Parameter{
			{Name: "ctx", Type: &model.NamedType{Package: "context", Type: "Context"}},
			{Name: "in", Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Input.GoIdent.GoImportPath), Type: m.Input.GoIdent.GoName}}},
		},
		Out: []*model.Parameter{
			{Type: &model.NamedType{Type: clientIfaceName}},
			{Type: model.PredeclaredType("error")},
		},
		Variadic: &model.Parameter{Name: "opts", Type: &model.NamedType{Package: "google.golang.org/grpc", Type: "CallOption"}},
	}
	serverMethod := &model.Method{
		Name: m.GoName,
		In: []*model.Parameter{
			{Name: "blob", Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Input.GoIdent.GoImportPath), Type: m.Input.GoIdent.GoName}}},
			{Name: "server", Type: &model.NamedType{Type: serverIfaceName}},
		},
		Out: []*model.Parameter{
			{Type: model.PredeclaredType("error")},
		},
	}
	clientIface := &model.Interface{
		Name:    clientIfaceName,
		Methods: baseClientStreamMethods(),
	}
	clientIface.AddMethod(&model.Method{
		Name: "Recv",
		Out: []*model.Parameter{
			{Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Output.GoIdent.GoImportPath), Type: m.Output.GoIdent.GoName}}},
			{Type: model.PredeclaredType("error")},
		},
	})
	serverIface := &model.Interface{
		Name:    serverIfaceName,
		Methods: baseServerStreamMethods(),
	}
	serverIface.AddMethod(&model.Method{
		Name: "Send",
		In: []*model.Parameter{
			{Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Output.GoIdent.GoImportPath), Type: m.Output.GoIdent.GoName}}},
		},
		Out: []*model.Parameter{
			{Type: model.PredeclaredType("error")},
		},
	})

	return clientMethod, serverMethod, []*model.Interface{clientIface, serverIface}
}

func makeClientStreamMethods(m *protogen.Method) (*model.Method, *model.Method, []*model.Interface) {
	clientIfaceName := fmt.Sprintf("%s_%sClient", m.Parent.GoName, m.GoName)
	serverIfaceName := fmt.Sprintf("%s_%sServer", m.Parent.GoName, m.GoName)
	clientMethod := &model.Method{
		Name: m.GoName,
		In: []*model.Parameter{
			{Name: "ctx", Type: &model.NamedType{Package: "context", Type: "Context"}},
		},
		Out: []*model.Parameter{
			{Type: &model.NamedType{Type: clientIfaceName}},
			{Type: model.PredeclaredType("error")},
		},
		Variadic: &model.Parameter{Name: "opts", Type: &model.NamedType{Package: "google.golang.org/grpc", Type: "CallOption"}},
	}
	serverMethod := &model.Method{
		Name: m.GoName,
		In: []*model.Parameter{
			{Name: "server", Type: &model.NamedType{Type: serverIfaceName}},
		},
		Out: []*model.Parameter{
			{Type: model.PredeclaredType("error")},
		},
	}
	clientIface := &model.Interface{
		Name:    clientIfaceName,
		Methods: baseClientStreamMethods(),
	}
	clientIface.AddMethod(&model.Method{
		Name: "Send",
		In: []*model.Parameter{
			{Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Input.GoIdent.GoImportPath), Type: m.Input.GoIdent.GoName}}},
		},
		Out: []*model.Parameter{
			{Type: model.PredeclaredType("error")},
		},
	})
	clientIface.AddMethod(&model.Method{
		Name: "CloseAndRecv",
		Out: []*model.Parameter{
			{Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Output.GoIdent.GoImportPath), Type: m.Output.GoIdent.GoName}}},
			{Type: model.PredeclaredType("error")},
		},
	})
	serverIface := &model.Interface{
		Name:    serverIfaceName,
		Methods: baseServerStreamMethods(),
	}
	serverIface.AddMethod(&model.Method{
		Name: "SendAndClose",
		In: []*model.Parameter{
			{Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Input.GoIdent.GoImportPath), Type: m.Input.GoIdent.GoName}}},
		},
		Out: []*model.Parameter{
			{Type: model.PredeclaredType("error")},
		},
	})
	serverIface.AddMethod(&model.Method{
		Name: "Recv",
		Out: []*model.Parameter{
			{Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Output.GoIdent.GoImportPath), Type: m.Output.GoIdent.GoName}}},
			{Type: model.PredeclaredType("error")},
		},
	})

	return clientMethod, serverMethod, []*model.Interface{clientIface, serverIface}
}

func makeBidirectionalStreamMethods(m *protogen.Method) (*model.Method, *model.Method, []*model.Interface) {
	clientIfaceName := fmt.Sprintf("%s_%sClient", m.Parent.GoName, m.GoName)
	serverIfaceName := fmt.Sprintf("%s_%sServer", m.Parent.GoName, m.GoName)
	clientMethod := &model.Method{
		Name: m.GoName,
		In: []*model.Parameter{
			{Name: "ctx", Type: &model.NamedType{Package: "context", Type: "Context"}},
		},
		Out: []*model.Parameter{
			{Type: &model.NamedType{Type: clientIfaceName}},
			{Type: model.PredeclaredType("error")},
		},
		Variadic: &model.Parameter{Name: "opts", Type: &model.NamedType{Package: "google.golang.org/grpc", Type: "CallOption"}},
	}
	serverMethod := &model.Method{
		Name: m.GoName,
		In: []*model.Parameter{
			{Name: "server", Type: &model.NamedType{Type: serverIfaceName}},
		},
		Out: []*model.Parameter{
			{Type: model.PredeclaredType("error")},
		},
	}
	clientIface := &model.Interface{
		Name:    clientIfaceName,
		Methods: baseClientStreamMethods(),
	}
	clientIface.AddMethod(&model.Method{
		Name: "Send",
		In: []*model.Parameter{
			{Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Input.GoIdent.GoImportPath), Type: m.Input.GoIdent.GoName}}},
		},
		Out: []*model.Parameter{
			{Type: model.PredeclaredType("error")},
		},
	})
	clientIface.AddMethod(&model.Method{
		Name: "Recv",
		Out: []*model.Parameter{
			{Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Output.GoIdent.GoImportPath), Type: m.Output.GoIdent.GoName}}},
			{Type: model.PredeclaredType("error")},
		},
	})
	serverIface := &model.Interface{
		Name:    serverIfaceName,
		Methods: baseServerStreamMethods(),
	}
	serverIface.AddMethod(&model.Method{
		Name: "Send",
		In: []*model.Parameter{
			{Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Input.GoIdent.GoImportPath), Type: m.Input.GoIdent.GoName}}},
		},
		Out: []*model.Parameter{
			{Type: model.PredeclaredType("error")},
		},
	})
	serverIface.AddMethod(&model.Method{
		Name: "Recv",
		Out: []*model.Parameter{
			{Type: &model.PointerType{Type: &model.NamedType{Package: string(m.Output.GoIdent.GoImportPath), Type: m.Output.GoIdent.GoName}}},
			{Type: model.PredeclaredType("error")},
		},
	})

	return clientMethod, serverMethod, []*model.Interface{clientIface, serverIface}
}

func baseClientStreamMethods() []*model.Method {
	return []*model.Method{
		{
			Name: "Header",
			Out: []*model.Parameter{
				{Type: &model.NamedType{Package: "google.golang.org/grpc/metadata", Type: "MD"}},
				{Type: model.PredeclaredType("error")},
			},
		},
		{
			Name: "Trailer",
			Out: []*model.Parameter{
				{Type: &model.NamedType{Package: "google.golang.org/grpc/metadata", Type: "MD"}},
			},
		},
		{
			Name: "CloseSend",
			Out: []*model.Parameter{
				{Type: model.PredeclaredType("error")},
			},
		},
		{
			Name: "Context",
			Out: []*model.Parameter{
				{Type: &model.NamedType{Package: "context", Type: "Context"}},
			},
		},
		{
			Name: "SendMsg",
			In: []*model.Parameter{
				{Name: "arg0", Type: model.PredeclaredType("interface{}")},
			},
			Out: []*model.Parameter{
				{Type: model.PredeclaredType("error")},
			},
		},
		{
			Name: "RecvMsg",
			In: []*model.Parameter{
				{Name: "arg0", Type: model.PredeclaredType("interface{}")},
			},
			Out: []*model.Parameter{
				{Type: model.PredeclaredType("error")},
			},
		},
	}
}

func baseServerStreamMethods() []*model.Method {
	return []*model.Method{
		{
			Name: "SetHeader",
			In: []*model.Parameter{
				{Type: &model.NamedType{Package: "google.golang.org/grpc/metadata", Type: "MD"}},
			},
			Out: []*model.Parameter{
				{Type: model.PredeclaredType("error")},
			},
		},
		{
			Name: "SendHeader",
			In: []*model.Parameter{
				{Type: &model.NamedType{Package: "google.golang.org/grpc/metadata", Type: "MD"}},
			},
			Out: []*model.Parameter{
				{Type: model.PredeclaredType("error")},
			},
		},
		{
			Name: "SetTrailer",
			In: []*model.Parameter{
				{Type: &model.NamedType{Package: "google.golang.org/grpc/metadata", Type: "MD"}},
			},
		},
		{
			Name: "Context",
			Out: []*model.Parameter{
				{Type: &model.NamedType{Package: "context", Type: "Context"}},
			},
		},
		{
			Name: "SendMsg",
			In: []*model.Parameter{
				{Name: "arg0", Type: model.PredeclaredType("interface{}")},
			},
			Out: []*model.Parameter{
				{Type: model.PredeclaredType("error")},
			},
		},
		{
			Name: "RecvMsg",
			In: []*model.Parameter{
				{Name: "arg0", Type: model.PredeclaredType("interface{}")},
			},
			Out: []*model.Parameter{
				{Type: model.PredeclaredType("error")},
			},
		},
	}
}

func main() {

	// If ParamFunc is non-nil, it will be called with each unknown
	// generator parameter.
	//
	// Plugins for protoc can accept parameters from the command line,
	// passed in the --<lang>_out protoc, separated from the output
	// directory with a colon; e.g.,
	//
	//   --go_out=<param1>=<value1>,<param2>=<value2>:<output_directory>
	//
	// Parameters passed in this fashion as a comma-separated list of
	// key=value pairs will be passed to the ParamFunc.
	//
	// The (flag.FlagSet).Set method matches this function signature,
	// so parameters can be converted into flags as in the following:
	//
	//   var flags flag.FlagSet
	//   value := flags.Bool("param", false, "")
	//   opts := &protogen.Options{
	//     ParamFunc: flags.Set,
	//   }
	//   protogen.Run(opts, func(p *protogen.Plugin) error {
	//     if *value { ... }
	//   })

	var (
		flags flag.FlagSet
		_     = flags.String("outfolder", "", "go grpc mock output folder")
		_     = flags.String("module", "", "go grpc mock module name")
	)

	protogen.Options{
		ParamFunc: flags.Set,
	}.Run(func(plugin *protogen.Plugin) error {
		plugin.SupportedFeatures = uint64(pluginpb.CodeGeneratorResponse_FEATURE_PROTO3_OPTIONAL)
		// fmt.Println("outfolder option: " + *outputFolder)

		for path, file := range plugin.FilesByPath {
			if !file.Generate {
				continue
			}
			pkg := fileToModel(file)
			if len(pkg.Interfaces) == 0 {
				continue
			}

			g := new(generator)
			g.filename = path

			if err := g.Generate(pkg, string(file.GoPackageName), string(file.GoImportPath)); err != nil {
				return err
			}
			grpcMockFileName := transformInput(file.GeneratedFilenamePrefix)

			if _, err := plugin.NewGeneratedFile(
				grpcMockFileName,
				file.GoImportPath,
			).Write(g.Output()); err != nil {
				return err
			}
		}
		return nil
	})
}

func transformInput(input string) string {
	// Split the input string by "/"
	parts := strings.Split(input, "/")

	// Extract the last part (service name) and convert it to the desired format
	serviceName := parts[len(parts)-1]
	serviceName = strings.ReplaceAll(serviceName, "-", "_") + "_go_grpc_mock.pb.go"

	// Replace the last part in the parts slice with the transformed service name
	parts[len(parts)-1] = serviceName

	// Join the parts back together to form the output string
	output := strings.Join(parts, "/")

	return output
}
