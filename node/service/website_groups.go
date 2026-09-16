// SPDX-License-Identifier: GPL-3.0-only
// Copyright (c) 2026 WorkMesh contributors

package service

import (
	"errors"
	"time"
)

// SetGroups 批量更新网站分组并一次持久化。
func (s *WebsiteService) SetGroups(ids []uint, groupID uint) error {
	if len(ids) == 0 || groupID == 0 {
		return errors.New("网站分组参数无效")
	}
	wanted := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	updated := 0
	for i := range s.websites {
		if _, ok := wanted[s.websites[i].ID]; ok {
			s.websites[i].WebsiteGroupID = groupID
			s.websites[i].UpdatedAt = time.Now().UTC()
			updated++
		}
	}
	if updated != len(wanted) {
		return errors.New("部分网站不存在")
	}
	return s.persist("websites", s.websites)
}

// GroupInUse 判断分组是否仍被网站引用。
func (s *WebsiteService) GroupInUse(groupID uint) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, website := range s.websites {
		if website.WebsiteGroupID == groupID {
			return true
		}
	}
	return false
}
