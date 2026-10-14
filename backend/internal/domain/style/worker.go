package style

import (
	"context"
	"log"
	"time"
)

// Worker 定时扫描候选用户并执行风格分析。
// 频率与触发阈值（triggerNewMessages）共同控制 LLM 成本。
type Worker struct {
	svc      *Service
	interval time.Duration
}

func NewWorker(svc *Service, interval time.Duration) *Worker {
	return &Worker{svc: svc, interval: interval}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	log.Printf("[style] worker started (interval %s, trigger >= %d new messages)", w.interval, triggerNewMessages)
	for {
		select {
		case <-ctx.Done():
			log.Println("[style] worker stopped")
			return
		case <-ticker.C:
			w.once(ctx)
		}
	}
}

func (w *Worker) once(ctx context.Context) {
	users, err := w.svc.repo.CandidateUsers(ctx, triggerNewMessages)
	if err != nil {
		log.Printf("[style] find candidates: %v", err)
		return
	}
	for _, userID := range users {
		if err := w.svc.AnalyzeUser(ctx, userID); err != nil {
			log.Printf("[style] analyze user %d: %v", userID, err)
			continue // 单个用户失败不影响其他用户
		}
	}
}
