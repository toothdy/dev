package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"sort"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnservice"
	baseservice "github.com/toothdy/cool-admin-go-next/modules/base/service"
	"github.com/toothdy/cool-admin-go-next/modules/space"
	"github.com/toothdy/cool-admin-go-next/modules/space/entity"
)

// InfoService 文件空间信息业务服务
type InfoService struct {
	*gnservice.Base[entity.Info, uint64]
	upload *baseservice.UploadService
	config space.Config
}

// NewInfo 文件空间信息业务服务
func NewInfo(
	infoBase *gnservice.Base[entity.Info, uint64],
	upload *baseservice.UploadService,
	config space.Config,
) (*InfoService, error) {
	if infoBase == nil || infoBase.Descriptor() == nil || upload == nil {
		return nil, exception.Core("文件空间信息服务依赖无效")
	}

	return &InfoService{Base: infoBase, upload: upload, config: config}, nil
}

// Add 新增文件信息并规范本地文件位置
func (service *InfoService) Add(
	ctx context.Context,
	input gnservice.AddInput[entity.Info],
) (gnservice.AddResult[uint64], error) {
	values := input.Many()
	if !input.IsMany() {
		values = []*gnservice.Mutable[entity.Info]{input.One()}
	}
	for _, value := range values {
		if value == nil {
			return gnservice.AddResult[uint64]{}, exception.Validate("文件信息无效")
		}
		rawURL, exists := value.Get("url")
		if !exists {
			continue
		}
		fileURL, matches := rawURL.(string)
		if !matches {
			return gnservice.AddResult[uint64]{}, exception.Validate("文件地址无效")
		}
		location, isManaged := service.upload.ResolveManagedURL(fileURL)
		if isManaged {
			if err := value.Set("key", location.Key); err != nil {
				return gnservice.AddResult[uint64]{}, err
			}
		}
	}

	return service.Base.Add(ctx, input)
}

// Delete 删除文件信息并按配置删除本地真实文件
func (service *InfoService) Delete(ctx context.Context, input gnservice.DeleteInput[uint64]) error {
	if !service.config.ShouldDeletePhysicalFile {
		return service.Base.Delete(ctx, input)
	}
	locations, err := service.managedLocations(ctx, input.IDs())
	if err != nil {
		return err
	}
	if err = service.Base.Delete(ctx, input); err != nil {
		return err
	}

	return removeManagedFiles(locations)
}

func (service *InfoService) managedLocations(
	ctx context.Context,
	ids []uint64,
) ([]baseservice.ManagedUploadLocation, error) {
	model, err := service.Base.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		URL string `orm:"url"`
	}
	if err = model.Fields("url").WhereIn("id", ids).Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询待删除文件失败")
	}
	locations := make(map[string]baseservice.ManagedUploadLocation, len(rows))
	for _, row := range rows {
		location, isManaged := service.upload.ResolveManagedURL(row.URL)
		if isManaged {
			locations[location.Root+"\x00"+location.RelativePath] = location
		}
	}
	keys := make([]string, 0, len(locations))
	for key := range locations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]baseservice.ManagedUploadLocation, len(keys))
	for index, key := range keys {
		result[index] = locations[key]
	}

	return result, nil
}

func removeManagedFiles(locations []baseservice.ManagedUploadLocation) error {
	var (
		root     *os.Root
		rootPath string
	)
	defer func() {
		if root != nil {
			_ = root.Close()
		}
	}()
	for _, location := range locations {
		if root == nil || rootPath != location.Root {
			if root != nil {
				_ = root.Close()
			}
			current, err := os.OpenRoot(location.Root)
			if errors.Is(err, fs.ErrNotExist) {
				root = nil
				rootPath = location.Root
				continue
			}
			if err != nil {
				return exception.WrapCore(err, "删除本地文件失败")
			}
			root = current
			rootPath = location.Root
		}
		file, err := root.Lstat(location.RelativePath)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return exception.WrapCore(err, "删除本地文件失败")
		}
		if !file.Mode().IsRegular() {
			return exception.Core("删除本地文件失败")
		}
		if err = root.Remove(location.RelativePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return exception.WrapCore(err, "删除本地文件失败")
		}
	}

	return nil
}
