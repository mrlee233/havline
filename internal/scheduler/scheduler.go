package scheduler

import (
	"context"
	"log"
	"time"
)

type Job struct {
	Name     string
	Interval time.Duration
	Run      func(ctx context.Context)
}

type Scheduler struct {
	jobs []Job
}

// New 会丢掉间隔非正的任务：time.NewTicker 对非正间隔直接 panic，这类任务应由调用方单独启动。
func New(jobs ...Job) *Scheduler {
	valid := make([]Job, 0, len(jobs))
	for _, job := range jobs {
		if job.Interval <= 0 {
			log.Printf("scheduler: 任务 %s 间隔为 %v，已跳过", job.Name, job.Interval)
			continue
		}
		valid = append(valid, job)
	}
	return &Scheduler{jobs: valid}
}

func (s *Scheduler) Start(ctx context.Context) {
	for _, job := range s.jobs {
		go s.runJob(ctx, job)
	}
}

func (s *Scheduler) runJob(ctx context.Context, job Job) {
	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()
	// 单个任务 panic 不能带走整个进程：包一层 recover，记录后继续下一轮
	run := func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("scheduler: 任务 %s panic: %v", job.Name, r)
			}
		}()
		job.Run(ctx)
	}
	run()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
