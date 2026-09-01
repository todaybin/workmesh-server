package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Group 与旧 Agent 的分组 DTO 保持兼容，网站和主机等资源通过 Type 区分。
type Group struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	IsDefault bool   `json:"isDefault"`
	IsDelete  bool   `json:"isDelete"`
}

type GroupService struct {
	mu     sync.RWMutex
	path   string
	groups []Group
}

func NewGroupService(root string) *GroupService {
	if strings.TrimSpace(root) == "" {
		root = os.Getenv("WORKMESH_DATA_DIR")
	}
	if strings.TrimSpace(root) == "" {
		root = "./data"
	}
	s := &GroupService{path: filepath.Join(root, "groups.json")}
	if b, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(b, &s.groups)
	}
	if len(s.groups) == 0 {
		s.groups = []Group{{ID: 1, Name: "Default", Type: "website", IsDefault: true}, {ID: 2, Name: "Default", Type: "host", IsDefault: true}}
		_ = s.persistLocked()
	}
	return s
}

func (s *GroupService) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.groups, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err = os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err = os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (s *GroupService) List(kind string) []Group {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Group, 0, len(s.groups))
	for _, g := range s.groups {
		if kind == "" || g.Type == kind {
			result = append(result, g)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].IsDefault != result[j].IsDefault {
			return result[i].IsDefault
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func (s *GroupService) Upsert(id uint, name, kind string, isDefault bool) (Group, error) {
	name, kind = strings.TrimSpace(name), strings.TrimSpace(kind)
	if name == "" || kind == "" || len(name) > 64 {
		return Group{}, errors.New("分组参数无效")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.groups {
		if s.groups[i].Type == kind && strings.EqualFold(s.groups[i].Name, name) && s.groups[i].ID != id {
			return Group{}, errors.New("分组名称已存在")
		}
	}
	if isDefault {
		for i := range s.groups {
			if s.groups[i].Type == kind {
				s.groups[i].IsDefault = false
			}
		}
	}
	if id == 0 {
		var max uint
		for _, g := range s.groups {
			if g.ID > max {
				max = g.ID
			}
		}
		id = max + 1
	}
	for i := range s.groups {
		if s.groups[i].ID == id {
			s.groups[i].Name, s.groups[i].Type, s.groups[i].IsDefault = name, kind, isDefault
			return s.groups[i], s.persistLocked()
		}
	}
	g := Group{ID: id, Name: name, Type: kind, IsDefault: isDefault}
	s.groups = append(s.groups, g)
	return g, s.persistLocked()
}

func (s *GroupService) Delete(id uint, inUse func(uint) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, g := range s.groups {
		if g.ID != id {
			continue
		}
		if g.IsDefault {
			return errors.New("默认分组不可删除")
		}
		if inUse != nil && inUse(id) {
			return errors.New("分组正在被网站使用")
		}
		s.groups = append(s.groups[:i], s.groups[i+1:]...)
		return s.persistLocked()
	}
	return fmt.Errorf("分组不存在")
}

// Touch 保留更新时间语义，便于审计和迁移时判断数据是否刷新。
func (s *GroupService) Touch() time.Time { return time.Now().UTC() }
