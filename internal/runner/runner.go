package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"auto-uooc-go/internal/api"
	"auto-uooc-go/internal/video"
)

const (
	defaultReportInterval  = 20 * time.Second
	defaultSpeedMultiplier = 2.0
	maxTailRetries         = 3
)

type clientAPI interface {
	GetUser(ctx context.Context) (api.User, error)
	GetCatalogList(ctx context.Context, cid int64) ([]api.CatalogNode, error)
	GetCourseProgress(ctx context.Context, cid int64) (api.CourseProgress, error)
	GetUnitLearn(ctx context.Context, cid, chapterID, sectionID int64) ([]api.UnitItem, error)
	MarkVideoLearn(ctx context.Context, req api.MarkVideoRequest) (api.MarkVideoResp, error)
}

type Options struct {
	DryRun          bool
	Concurrency     int
	ReportInterval  time.Duration
	SpeedMultiplier float64
	FFProbePath     string
}

type Runner struct {
	client clientAPI
	opts   Options
}

type videoTask struct {
	CID        int64
	ChapterID  int64
	SectionID  int64
	ResourceID int64
	Title      string
	SourceURL  string
}

type taskResult struct {
	Task   videoTask
	Status string
	Err    error
}

type discoveryResult struct {
	Tasks      []videoTask
	GateLocked bool
}

func New(client clientAPI, opts Options) *Runner {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 3
	}
	if opts.ReportInterval <= 0 {
		opts.ReportInterval = defaultReportInterval
	}
	if opts.SpeedMultiplier <= 0 {
		opts.SpeedMultiplier = defaultSpeedMultiplier
	}
	video.ConfigureFFProbePath(opts.FFProbePath)
	return &Runner{client: client, opts: opts}
}

func (r *Runner) RunCourse(ctx context.Context, cid int64) error {
	user, err := r.client.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("login validation failed: %w", err)
	}
	fmt.Printf("login ok, uid=%d, nick=%s\n", user.ID, user.Nick)

	progress, err := r.client.GetCourseProgress(ctx, cid)
	if err != nil {
		return fmt.Errorf("get course progress: %w", err)
	}
	if progress.VideoProgressValue >= 100 || strings.TrimSpace(progress.VideoProgress) == "100%" {
		fmt.Println("video progress already full (100%), no update needed")
		return nil
	}
	fmt.Printf("current video progress: %s\n", progress.VideoProgress)

	processed := make(map[int64]struct{})
	var allResults []taskResult
	everGateLocked := false
	round := 1

	for {
		catalog, err := r.client.GetCatalogList(ctx, cid)
		if err != nil {
			return fmt.Errorf("get catalog list: %w", err)
		}
		discovery := r.discoverVideoTasks(ctx, cid, catalog)
		if discovery.GateLocked {
			everGateLocked = true
		}

		pending := filterUnprocessedTasks(discovery.Tasks, processed)
		if len(pending) == 0 {
			fmt.Printf("round %d: no new video tasks found, stop\n", round)
			break
		}

		fmt.Printf("round %d: discovered %d new video tasks\n", round, len(pending))
		results := r.processTasks(ctx, pending)
		allResults = append(allResults, results...)

		for _, t := range pending {
			processed[t.ResourceID] = struct{}{}
		}

		fmt.Printf("round %d completed, re-checking for newly unlocked videos...\n", round)
		round++
	}

	if len(allResults) == 0 {
		fmt.Println("no video tasks found")
		if everGateLocked {
			fmt.Println("course has gate-locked sections (code=600), and no new videos unlocked yet")
		}
		return nil
	}

	return summarize(allResults, everGateLocked)
}

func filterUnprocessedTasks(tasks []videoTask, processed map[int64]struct{}) []videoTask {
	out := make([]videoTask, 0, len(tasks))
	for _, t := range tasks {
		if _, ok := processed[t.ResourceID]; ok {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (r *Runner) discoverVideoTasks(ctx context.Context, cid int64, catalog []api.CatalogNode) discoveryResult {
	var tasks []videoTask
	gateLocked := false
	for _, chapter := range catalog {
		sections := chapter.Children
		if len(sections) == 0 {
			sections = []api.CatalogNode{chapter}
		}
		for _, section := range sections {
			items, err := r.client.GetUnitLearn(ctx, cid, chapter.ID, section.ID)
			if err != nil {
				if api.IsAPIErrorCode(err, 600) {
					fmt.Printf("pause discovery: locked section chapter=%d section=%d\n", chapter.ID, section.ID)
					gateLocked = true
					return discoveryResult{Tasks: tasks, GateLocked: gateLocked}
				}
				fmt.Printf("skip section: chapter=%d section=%d error=%v\n", chapter.ID, section.ID, err)
				continue
			}
			for _, item := range items {
				source := chooseSource(item)
				if source == "" {
					continue
				}
				if item.Finished != 0 {
					fmt.Printf("skip done task: [%d] %s (finished=%d)\n", item.ID, item.Title, item.Finished)
					continue
				}
				tasks = append(tasks, videoTask{
					CID:        cid,
					ChapterID:  chapter.ID,
					SectionID:  section.ID,
					ResourceID: item.ID,
					Title:      item.Title,
					SourceURL:  source,
				})
			}
		}
	}
	return discoveryResult{Tasks: tasks, GateLocked: gateLocked}
}

func chooseSource(item api.UnitItem) string {
	for _, s := range item.VideoPlayList {
		if strings.TrimSpace(s.Source) != "" {
			return strings.TrimSpace(s.Source)
		}
	}
	for _, s := range item.VideoURL {
		if strings.TrimSpace(s.Source) != "" {
			return strings.TrimSpace(s.Source)
		}
	}
	return ""
}

func (r *Runner) processTasks(ctx context.Context, tasks []videoTask) []taskResult {
	sem := make(chan struct{}, r.opts.Concurrency)
	results := make(chan taskResult, len(tasks))
	var wg sync.WaitGroup

	for _, task := range tasks {
		t := task
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res := r.handleTask(ctx, t)
			results <- res
		}()
	}

	wg.Wait()
	close(results)

	out := make([]taskResult, 0, len(tasks))
	for res := range results {
		out = append(out, res)
	}
	return out
}

func (r *Runner) handleTask(ctx context.Context, task videoTask) taskResult {
	fmt.Printf("start task: [%d] %s\n", task.ResourceID, task.Title)
	duration, err := video.FetchDurationSeconds(ctx, task.SourceURL)
	if err != nil || duration <= 0 {
		if err == nil {
			err = errors.New("invalid duration")
		}
		fmt.Printf("skip task: [%d] cannot get video duration: %v\n", task.ResourceID, err)
		return taskResult{Task: task, Status: "skipped", Err: err}
	}

	if duration <= 1 {
		return taskResult{Task: task, Status: "skipped", Err: fmt.Errorf("duration too short: %.2f", duration)}
	}

	tailRetries := 0
	pos := 0.0
	firstReport := true
	progressStep := r.opts.ReportInterval.Seconds() * r.opts.SpeedMultiplier
	if progressStep <= 0 {
		progressStep = defaultReportInterval.Seconds() * defaultSpeedMultiplier
	}
	for {
		nextPos := 0.0
		if firstReport {
			firstReport = false
		} else {
			nextPos = pos + progressStep
			if nextPos > duration-1 {
				nextPos = duration - 1
			}
		}

		if r.opts.DryRun {
			fmt.Printf("dry-run task [%d]: chapter=%d section=%d video_length=%.2f video_pos=%.2f\n", task.ResourceID, task.ChapterID, task.SectionID, duration, nextPos)
		} else {
			resp, err := r.client.MarkVideoLearn(ctx, api.MarkVideoRequest{
				ChapterID:   task.ChapterID,
				CID:         task.CID,
				ResourceID:  task.ResourceID,
				SectionID:   task.SectionID,
				VideoLength: duration,
				VideoPos:    nextPos,
			})
			if err != nil {
				if api.IsAPIErrorCode(err, 600) {
					fmt.Printf("skip locked task: [%d] %s reason=%v\n", task.ResourceID, task.Title, err)
					return taskResult{Task: task, Status: "skipped", Err: err}
				}
				fmt.Printf("task [%d] report failed: %v\n", task.ResourceID, err)
				return taskResult{Task: task, Status: "failed", Err: err}
			}
			fmt.Printf("task [%d] report ok: pos %.2f/%.2f finished=%v\n", task.ResourceID, nextPos, duration, resp.Finished != 0)
			if resp.Finished != 0 {
				return taskResult{Task: task, Status: "done"}
			}
		}

		if nextPos >= duration-1 {
			tailRetries++
			if r.opts.DryRun {
				return taskResult{Task: task, Status: "dry-run"}
			}
			if tailRetries >= maxTailRetries {
				return taskResult{Task: task, Status: "failed", Err: fmt.Errorf("tail retries exceeded")}
			}
		} else {
			tailRetries = 0
		}

		pos = nextPos
		time.Sleep(r.opts.ReportInterval)
	}
}

func summarize(results []taskResult, gateLocked bool) error {
	var done, skipped, failed, dry int
	var errs []error

	for _, r := range results {
		switch r.Status {
		case "done":
			done++
		case "dry-run":
			dry++
		case "skipped":
			skipped++
		case "failed":
			failed++
			if r.Err != nil {
				errs = append(errs, fmt.Errorf("resource %d: %w", r.Task.ResourceID, r.Err))
			}
		}
	}

	fmt.Printf("summary: total=%d done=%d dry-run=%d skipped=%d failed=%d\n", len(results), done, dry, skipped, failed)
	if gateLocked {
		fmt.Println("\n** Course has gate-locked sections. Please complete the quizzes in unlocked sections, then re-run this command. **")
	}
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Printf("error: %v\n", err)
		}
		return fmt.Errorf("%d task(s) failed", len(errs))
	}
	return nil
}
