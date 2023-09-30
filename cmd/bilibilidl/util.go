package main

import (
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"path/filepath"
)

func getVideoLocation() string {
	location := "videos"

	if PathExists(location) {
		return location
	}

	err := os.MkdirAll(filepath.Dir(location), os.ModePerm)
	if err != nil {
		panic(err)
	}

	return location
}

func PathExists(p string) bool {
	ok, _ := PathExistsWithError(p)
	return ok
}

func PathExistsWithError(p string) (bool, error) {
	_, err := os.Stat(p)
	return err == nil || os.IsExist(err), err
}

func MarshalYaml[T any](t T) string {
	content, err := yaml.Marshal(t)
	if err != nil {
		log.Printf("序列化Yaml失败：%s", err)
		return ""
	}

	return string(content)
}

func UnmarshalYaml[T any](content string) (t *T, ok bool) {
	err := yaml.Unmarshal([]byte(content), &t)
	if err != nil {
		log.Printf("解析%T类型YAML失败：%s\nerror:\n%s", t, content, err)
		return nil, false
	}

	return t, true
}

func LoadContent(path string) (string, bool) {
	if _, err := os.Stat(path); err != nil {
		return "", false
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		panic(err)
		return "", false
	}

	return string(bytes), true
}

func WriteContent(filePath string, content string) {
	dirPath := filepath.Dir(filePath)

	// 创建目录，包括任何必需的父目录，权限设置为 0755
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		panic(err)
	}

	// 创建新的文件
	file, err := os.Create(filePath)
	if err != nil {
		panic(err)
	}

	defer func(f *os.File) {
		log.Printf("[文件保存]写入并保存文件: %s", f.Name())
		err := f.Close()
		if err != nil {
			panic(err)
		}
	}(file)

	// 将内容写入文件
	if _, err := file.WriteString(content); err != nil {
		panic(err)
	}
}
