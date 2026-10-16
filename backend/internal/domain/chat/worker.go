package chat

import (
	"context"
	"log"
	"time"
)

// MemoryWorker 定时扫描需要更新长期记忆的会话并执行合并。
// 与 style worker 同构：定时 + 增量阈值共同控制 LLM 成本。
type MemoryWorker struct {
	svc      *Service
	interval time.Duration
}

func NewMemoryWorker(svc *Service, interval time.Duration) *MemoryWorker {
	return &MemoryWorker{svc: svc, interval: interval}
}

func (w *MemoryWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	log.Printf("[chat] memory worker started (interval %s, trigger >= %d new messages)", w.interval, memoryTriggerNewMessages)
	for {
		select {
		case <-ctx.Done():
			log.Println("[chat] memory worker stopped")
			return
		case <-ticker.C:
			w.once(ctx)
		}
	}
}

func (w *MemoryWorker) once(ctx context.Context) {
	convs, err := w.svc.repo.MemoryCandidates(ctx, memoryTriggerNewMessages)
	if err != nil {
		log.Printf("[chat] memory candidates: %v", err)
		return
	}
	for _, conv := range convs {
		if err := w.svc.UpdateMemory(ctx, conv); err != nil {
			log.Printf("[chat] memory update conv %d: %v", conv.ID, err)
			continue // 单个失败不影响其他
		}
	}
}
