package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	st "github.com/syncthing/syncthing/proto/ext"

	"github.com/gogo/protobuf/vanity"
	"github.com/gogo/protobuf/vanity/command"

	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"

	"github.com/gogo/protobuf/gogoproto"

	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/protoc-gen-gogo/generator"
)

func main() {
	req := command.Read()
	files := req.GetProtoFile()
	files = vanity.FilterFiles(files, vanity.NotGoogleProtobufDescriptorProto)

	vanity.ForEachFile(files, vanity.TurnOffGoGettersAll)
	vanity.ForEachFile(files, TurnOnProtoSizerAll)
	vanity.ForEachFile(files, vanity.TurnOffGoEnumPrefixAll)
	vanity.ForEachFile(files, vanity.TurnOffGoUnrecognizedAll)
	vanity.ForEachFile(files, vanity.TurnOffGoUnkeyedAll)
	vanity.ForEachFile(files, vanity.TurnOffGoSizecacheAll)
	vanity.ForEachFieldInFiles(files, JsonLowerCamelCaseNaming)
	vanity.ForEachEnumInFiles(files, EnumCamelCaseNaming)
	vanity.ForEachFile(files, SetPackagePrefix("github.com/syncthing/syncthing"))

	vanity.ForEachFieldInFilesExcludingExtensions(files, TurnOffNullableForMessages)
	vanity.ForEachFieldInFiles(files, HandleCustomExtensions)

	resp := command.Generate(req)
	command.Write(resp)
}

func TurnOnProtoSizerAll(file *descriptor.FileDescriptorProto) {
	vanity.SetBoolFileOption(gogoproto.E_ProtosizerAll, true)(file)
}

func TurnOffNullableForMessages(field *descriptor.FieldDescriptorProto) {
	if field.IsMessage() && !vanity.FieldHasBoolExtension(field, gogoproto.E_Nullable) {
		vanity.SetBoolFieldOption(gogoproto.E_Nullable, false)(field)
	}
}

func EnumCamelCaseNaming(enum *descriptor.EnumDescriptorProto) {
	for _, field := range enum.Value {
		if field == nil {
			continue
		}
		if field.Options == nil {
			field.Options = &descriptor.EnumValueOptions{}
		}
		customName := gogoproto.GetEnumValueCustomName(field)
		if customName != "" {
			continue
		}
		SetStringFieldOption(gogoproto.E_EnumvalueCustomname, generator.CamelCase(*field.Name))
	}
}

func SetPackagePrefix(prefix string) func(file *descriptor.FileDescriptorProto) {
	return func(file *descriptor.FileDescriptorProto) {
		if file.Options.GoPackage == nil {
			pkg, _ := filepath.Split(file.GetName())
			fullPkg := prefix + "/" + strings.TrimSuffix(pkg, "/")
			file.Options.GoPackage = &fullPkg
		}
	}
}

func toLowerCamelCase(input string) string {
	upperCamel := []rune(generator.CamelCase(input))
	upperCamel[0] = unicode.ToLower(upperCamel[0])
	return string(upperCamel)
}

func JsonLowerCamelCaseNaming(field *descriptor.FieldDescriptorProto) {
	SetStringFieldOption(gogoproto.E_Jsontag, toLowerCamelCase(*field.Name))(field)
}

func SetStringFieldOption(extension *proto.ExtensionDesc, value string) func(field *descriptor.FieldDescriptorProto) {
	return func(field *descriptor.FieldDescriptorProto) {
		if _, ok := GetFieldStringExtension(field, extension); ok {
			return
		}
		if field.Options == nil {
			field.Options = &descriptor.FieldOptions{}
		}
		if err := proto.SetExtension(field.Options, extension, &value); err != nil {
			panic(err)
		}
	}
}

func GetFieldStringExtension(field *descriptor.FieldDescriptorProto, extension *proto.ExtensionDesc) (string, bool) {
	if field.Options == nil {
		return "", false
	}
	value, err := proto.GetExtension(field.Options, extension)
	if err != nil {
		return "", false
	}
	if value == nil {
		return "", false
	}
	if v, ok := value.(*string); !ok || v == nil {
		return "", false
	} else {
		return *v, true
	}
}

func GetFieldBooleanExtension(field *descriptor.FieldDescriptorProto, extension *proto.ExtensionDesc) (bool, bool) {
	if field.Options == nil {
		return false, false
	}
	value, err := proto.GetExtension(field.Options, extension)
	if err != nil {
		return false, false
	}
	if value == nil {
		return false, false
	}
	if v, ok := value.(*bool); !ok || v == nil {
		return false, false
	} else {
		return *v, true
	}
}

func HandleCustomExtensions(field *descriptor.FieldDescriptorProto) {
	if field.Options == nil {
		field.Options = &descriptor.FieldOptions{}
	}

	current := ""
	if v, ok := GetFieldStringExtension(field, gogoproto.E_Moretags); ok {
		current = v
	}

	if jsonValue, ok := GetFieldStringExtension(field, st.E_Json); ok {
		SetStringFieldOption(gogoproto.E_Jsontag, jsonValue)(field)
	}

	if len(current) > 0 {
		current += " "
	}
	if xmlValue, ok := GetFieldStringExtension(field, st.E_Xml); ok {
		current += fmt.Sprintf(`xml:"%s"`, xmlValue)
	} else {
		current += fmt.Sprintf(`xml:"%s"`, toLowerCamelCase(*field.Name))
	}
	if defaultValue, ok := GetFieldStringExtension(field, st.E_Default); ok {
		if len(current) > 0 {
			current += " "
		}
		current += fmt.Sprintf(`default:"%s"`, defaultValue)
	}
	if restartValue, ok := GetFieldBooleanExtension(field, st.E_Restart); ok {
		if len(current) > 0 {
			current += " "
		}
		current += fmt.Sprintf(`restart:"%t"`, restartValue)
	}

	SetStringFieldOption(gogoproto.E_Moretags, current)(field)
}
