package stats

import (
	"fmt"
	"reflect"
	"testing"
)

func TestPathsToFolderTreeStructureNested(t *testing.T) {
	paths := []string{"folder1/file1.jpg", "flower.jpg", "folder1/file2.jpg", "folder2/file1.jpg"}
	obj := pathsToFolderTreeStructure(paths)

	expected := FolderTreeStructure{
		Name:     "",
		IsFolder: true,
		Children: []FolderTreeStructure{
			{
				Name:     "folder1",
				IsFolder: true,
				Children: []FolderTreeStructure{
					{Name: "file1.jpg", IsFolder: false, Children: make([]FolderTreeStructure, 0)},
					{Name: "file2.jpg", IsFolder: false, Children: make([]FolderTreeStructure, 0)},
				},
			},
			{Name: "flower.jpg", IsFolder: false, Children: make([]FolderTreeStructure, 0)},
			{
				Name:     "folder2",
				IsFolder: true,
				Children: []FolderTreeStructure{
					{Name: "file1.jpg", IsFolder: false, Children: make([]FolderTreeStructure, 0)},
				},
			},
		},
	}

	fmt.Println(obj.Equal(expected))
	fmt.Println(reflect.DeepEqual(obj, expected))
}

func TestPathsToFolderTreeStructureNested2(t *testing.T) {
	paths := []string{"folder1/file1.jpg", "folder1/file2.jpg", "folder2/file1.jpg"}
	obj := pathsToFolderTreeStructure(paths)

	expected := FolderTreeStructure{
		Name:     "",
		IsFolder: true,
		Children: []FolderTreeStructure{
			{
				Name:     "folder1",
				IsFolder: true,
				Children: []FolderTreeStructure{
					{Name: "file1.jpg", IsFolder: false, Children: make([]FolderTreeStructure, 0)},
					{Name: "file2.jpg", IsFolder: false, Children: make([]FolderTreeStructure, 0)},
				},
			},
			{
				Name:     "folder2",
				IsFolder: true,
				Children: []FolderTreeStructure{
					{Name: "file1.jpg", IsFolder: false, Children: make([]FolderTreeStructure, 0)},
				},
			},
		},
	}

	fmt.Println(obj.Equal(expected))
	fmt.Println(reflect.DeepEqual(obj, expected))
}

func TestPathsToFolderTreeStructureNotNested(t *testing.T) {
	paths := []string{"file1.jpg", "flower.jpg", "file2.jpg", "file3.jpg"}
	obj := pathsToFolderTreeStructure(paths)

	expected := FolderTreeStructure{
		Name:     "",
		IsFolder: true,
		Children: []FolderTreeStructure{
			{Name: "file1.jpg", IsFolder: false, Children: make([]FolderTreeStructure, 0)},
			{Name: "flower.jpg", IsFolder: false, Children: make([]FolderTreeStructure, 0)},
			{Name: "file2.jpg", IsFolder: false, Children: make([]FolderTreeStructure, 0)},
			{Name: "file3.jpg", IsFolder: false, Children: make([]FolderTreeStructure, 0)},
		},
	}

	fmt.Println(obj.Equal(expected))
	fmt.Println(reflect.DeepEqual(obj, expected))
}
