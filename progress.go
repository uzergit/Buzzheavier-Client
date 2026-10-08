package main

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// progressReader counts bytes as the HTTP client reads the file and draws a
// progress line while the upload runs.
type progressReader struct {
	r     io.Reader
	total int64
	n     atomic.Int64
	start time.Time
	done  chan struct{}
	wg    sync.WaitGroup
}

func newProgress(r io.Reader, total int64) *progressReader {
	p := &progressReader{r: r, total: total, start: time.Now(), done: make(chan struct{})}
	p.wg.Add(1)
	go p.loop()
	return p
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.n.Add(int64(n))
	return n, err
}

func (p *progressReader) loop() {
	defer p.wg.Done()
	interval := 150 * time.Millisecond
	if !stdoutIsTTY {
		interval = 5 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-t.C:
			p.draw()
		}
	}
}

// Stop ends the progress line; ok selects whether to draw it as complete.
func (p *progressReader) Stop(ok bool) {
	close(p.done)
	p.wg.Wait()
	if ok {
		p.n.Store(p.total)
		p.draw()
	}
	if stdoutIsTTY {
		fmt.Println()
	}
}

func (p *progressReader) draw() {
	sent := p.n.Load()
	frac := 1.0
	if p.total > 0 {
		frac = float64(sent) / float64(p.total)
	}
	const width = 24
	filled := int(frac * width)
	if filled > width {
		filled = width
	}
	bar := green(strings.Repeat("█", filled)) + dim(strings.Repeat("░", width-filled))

	elapsed := time.Since(p.start).Seconds()
	speed := 0.0
	if elapsed > 0 {
		speed = float64(sent) / elapsed
	}
	tail := ""
	switch {
	case sent >= p.total:
		tail = "finishing…"
	case speed > 0:
		tail = "ETA " + formatETA(time.Duration(float64(p.total-sent)/speed*float64(time.Second)))
	}
	line := fmt.Sprintf("  %s %3.0f%%  %s / %s  %s/s  %s",
		bar, frac*100, humanBytes(sent), humanBytes(p.total), humanBytes(int64(speed)), tail)
	if stdoutIsTTY {
		fmt.Print("\r\033[K" + line)
	} else {
		fmt.Println(line)
	}
}

func formatETA(d time.Duration) string {
	d = d.Round(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
