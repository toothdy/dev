package artifact

import (
	"archive/zip"
	"bytes"
	"os"
	"testing"
)

func TestPackIsReproducibleAndReadable(t *testing.T) {
	files := validPackageFiles(t)
	first, err := Pack(files)
	if err != nil {
		t.Fatalf("首次打包失败: %v", err)
	}
	second, err := Pack(files)
	if err != nil {
		t.Fatalf("再次打包失败: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("相同文件两次打包结果不一致")
	}

	packageData, err := Read(first)
	if err != nil {
		t.Fatalf("读取生成的插件包失败: %v", err)
	}
	if packageData.Manifest.Key != "echo-plugin" {
		t.Fatalf("插件 key = %q", packageData.Manifest.Key)
	}
	if packageData.SHA256 == "" || packageData.Size != int64(len(first)) {
		t.Fatalf("制品摘要信息错误: %+v", packageData)
	}
	for name, content := range files {
		if !bytes.Equal(packageData.Files[name], content) {
			t.Fatalf("文件 %q 内容不一致", name)
		}
	}

	reader, err := zip.NewReader(bytes.NewReader(first), int64(len(first)))
	if err != nil {
		t.Fatalf("打开生成的 ZIP 失败: %v", err)
	}
	previous := ""
	for _, entry := range reader.File {
		if previous != "" && entry.Name < previous {
			t.Fatalf("ZIP 路径未排序: %q 位于 %q 后", entry.Name, previous)
		}
		if !entry.Modified.Equal(archiveTime) || entry.Mode().Perm() != 0o644 {
			t.Fatalf("ZIP 元数据不稳定: %q %v %o", entry.Name, entry.Modified, entry.Mode().Perm())
		}
		previous = entry.Name
	}
}

func TestReadRejectsUnsafeEntries(t *testing.T) {
	manifest := validManifestData(t)
	tests := map[string][]testZipEntry{
		"父目录穿越": {
			{name: "../plugin.json", content: manifest},
		},
		"绝对路径": {
			{name: "/plugin.json", content: manifest},
		},
		"反斜杠": {
			{name: `assets\logo.png`, content: []byte("logo")},
		},
		"非规范路径": {
			{name: "assets/../plugin.json", content: manifest},
		},
		"重复路径": {
			{name: ManifestFile, content: manifest},
			{name: ManifestFile, content: manifest},
		},
		"符号链接": {
			{name: "link", content: []byte("plugin.wasm"), mode: os.ModeSymlink | 0o777},
		},
	}

	for name, entries := range tests {
		t.Run(name, func(t *testing.T) {
			data := makeZip(t, entries)
			if _, err := Read(data); err == nil {
				t.Fatal("不安全 ZIP 条目未被拒绝")
			}
		})
	}
}

func TestReadRejectsMissingAndReferencedFiles(t *testing.T) {
	withoutModule := makeZip(t, []testZipEntry{{name: ManifestFile, content: validManifestData(t)}})
	if _, err := Read(withoutModule); err == nil {
		t.Fatal("缺少 WASM 的插件包未被拒绝")
	}

	withoutResources := makeZip(t, []testZipEntry{
		{name: ManifestFile, content: validManifestData(t)},
		{name: ModuleFile, content: []byte("wasm")},
	})
	if _, err := Read(withoutResources); err == nil {
		t.Fatal("缺少 Manifest 引用资源的插件包未被拒绝")
	}
}

func TestReadEnforcesLimits(t *testing.T) {
	files := validPackageFiles(t)
	files["assets/large.bin"] = bytes.Repeat([]byte("a"), 4096)
	data := makeZipFromFiles(t, files)

	limits := DefaultLimits()
	limits.MaxPackageBytes = int64(len(data) - 1)
	if _, err := ReadWithLimits(data, limits); err == nil {
		t.Fatal("超出压缩大小限制的插件包未被拒绝")
	}

	limits = DefaultLimits()
	limits.MaxUnpackedBytes = 1024
	if _, err := ReadWithLimits(data, limits); err == nil {
		t.Fatal("超出解压大小限制的插件包未被拒绝")
	}

	limits = DefaultLimits()
	limits.MaxEntries = 2
	if _, err := ReadWithLimits(data, limits); err == nil {
		t.Fatal("超出条目数限制的插件包未被拒绝")
	}
}

func TestPackRejectsInvalidInput(t *testing.T) {
	tests := []map[string][]byte{
		{ManifestFile: validManifestData(t)},
		{"../plugin.json": validManifestData(t), ModuleFile: []byte("wasm")},
	}
	for _, files := range tests {
		if _, err := Pack(files); err == nil {
			t.Fatal("非法打包输入未被拒绝")
		}
	}
}

type testZipEntry struct {
	name    string
	content []byte
	mode    os.FileMode
}

func validPackageFiles(t *testing.T) map[string][]byte {
	t.Helper()
	return map[string][]byte{
		ManifestFile:      validManifestData(t),
		ModuleFile:        {0x00, 0x61, 0x73, 0x6d},
		"README.md":       []byte("# Echo\n"),
		"assets/logo.png": []byte("png"),
	}
}

func makeZipFromFiles(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	entries := make([]testZipEntry, 0, len(files))
	for name, content := range files {
		entries = append(entries, testZipEntry{name: name, content: content})
	}

	return makeZip(t, entries)
}

func makeZip(t *testing.T, entries []testZipEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, value := range entries {
		header := &zip.FileHeader{Name: value.name, Method: zip.Deflate}
		mode := value.mode
		if mode == 0 {
			mode = 0o644
		}
		header.SetMode(mode)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("创建 ZIP 条目失败: %v", err)
		}
		if _, err := entry.Write(value.content); err != nil {
			t.Fatalf("写入 ZIP 条目失败: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭 ZIP 失败: %v", err)
	}

	return buffer.Bytes()
}
