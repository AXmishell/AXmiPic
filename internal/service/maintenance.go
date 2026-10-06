package service

import (
	"context"
	"time"

	"github.com/AXmishell/axmipic/internal/storage"
	"github.com/AXmishell/axmipic/internal/store"
)

// OrphanReconciler 对账存储对象与数据库记录，删除存储中存在、但数据库无对应
// 记录且已超过保留期的孤儿图片对象（例如上传成功后入库失败、或历史遗留）。
type OrphanReconciler struct {
	repo    *store.Repository
	manager *storage.Manager
	grace   time.Duration
}

// NewOrphanReconciler 构造一个 OrphanReconciler。grace 是对象的最短保留时长。
func NewOrphanReconciler(repo *store.Repository, manager *storage.Manager, grace time.Duration) *OrphanReconciler {
	return &OrphanReconciler{repo: repo, manager: manager, grace: grace}
}

// Reconcile 遍历所有已注册后端，删除早于 grace 且数据库无对应记录的图片对象，
// 返回删除的对象数量。无法枚举的后端会被跳过；单个后端的列举失败不会中断其余
// 后端的对账。
func (r *OrphanReconciler) Reconcile(ctx context.Context, now time.Time) (int, error) {
	// 安全阀：数据库没有任何图片记录但存储中仍有对象，通常是数据库被指向了
	// 错误位置；此时不对账，避免误删全部对象。
	stats, err := r.repo.ImageStats(ctx)
	if err != nil {
		return 0, err
	}
	if stats.Count == 0 {
		return 0, nil
	}

	cutoff := now.Add(-r.grace)
	removed := 0
	for _, backend := range r.manager.All() {
		lister, ok := backend.Storage.(storage.ObjectLister)
		if !ok {
			continue
		}
		objects, err := lister.List(ctx, "")
		if err != nil {
			return removed, err
		}
		candidates := make([]string, 0, len(objects))
		for i := range objects {
			if isOrphanCandidate(&objects[i], cutoff) {
				candidates = append(candidates, objects[i].Key)
			}
		}
		if len(candidates) == 0 {
			continue
		}
		existing, err := r.repo.ExistingImageKeys(ctx, candidates)
		if err != nil {
			return removed, err
		}
		for _, key := range candidates {
			if _, ok := existing[key]; ok {
				continue
			}
			if err := backend.Storage.Delete(ctx, key); err != nil {
				continue
			}
			removed++
		}
	}
	return removed, nil
}

// isOrphanCandidate 报告一个对象是否值得进一步检查：键必须符合本应用生成的
// 日期分区格式，且键中的日期与（若有）最后修改时间都早于 cutoff。这样可以
// 避免误删非本应用的对象或刚上传、尚未入库的对象。
func isOrphanCandidate(obj *storage.ObjectInfo, cutoff time.Time) bool {
	created, ok := dateFromKey(obj.Key)
	if !ok {
		return false
	}
	if !created.Before(cutoff) {
		return false
	}
	if !obj.LastModified.IsZero() && !obj.LastModified.Before(cutoff) {
		return false
	}
	return true
}

// dateFromKey 解析形如 `2006/01/02/<name>` 的键，返回其日期部分。
func dateFromKey(key string) (time.Time, bool) {
	if len(key) < 11 || key[10] != '/' {
		return time.Time{}, false
	}
	created, err := time.Parse("2006/01/02", key[:10])
	if err != nil {
		return time.Time{}, false
	}
	return created, true
}
