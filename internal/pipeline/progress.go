package pipeline

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

type Progress struct {
	totalBytes uint64
	doneBytes  atomic.Uint64
	start      time.Time
	lastPrint  time.Time
}

func NewProgress(totalBytes uint64) *Progress {
	return &Progress{
		totalBytes: totalBytes,
		start:      time.Now(),
		lastPrint:  time.Now(),
	}
}

func (p *Progress) AddDone(n uint64) {
	p.doneBytes.Add(n)
}

func (p *Progress) Done() uint64 {
	return p.doneBytes.Load()
}

func (p *Progress) Total() uint64 {
	return p.totalBytes
}

func (p *Progress) Percent() float64 {
	t := p.totalBytes
	if t == 0 {
		return 100
	}
	return float64(p.Done()) * 100 / float64(t)
}

func (p *Progress) SpeedBytesPerSec() float64 {
	el := time.Since(p.start).Seconds()
	if el <= 0 {
		return 0
	}
	return float64(p.Done()) / el
}

func (p *Progress) Eta() time.Duration {
	speed := p.SpeedBytesPerSec()
	if speed <= 0 {
		return 0
	}
	remain := float64(p.totalBytes - p.Done())
	return time.Duration(remain/speed) * time.Second
}

func (p *Progress) Render(w io.Writer, force bool) {
	now := time.Now()
	// не флудим: 5 раз в секунду максимум, если не force
	if !force && now.Sub(p.lastPrint) < 200*time.Millisecond {
		return
	}
	p.lastPrint = now

	done := p.Done()
	total := p.totalBytes
	pct := p.Percent()
	speed := p.SpeedBytesPerSec()
	eta := p.Eta()

	fmt.Fprintf(w,
		"\rProgress: %6.2f%%  %s / %s  %s/s  ETA %s",
		pct,
		humanBytes(done),
		humanBytes(total),
		humanBytes(uint64(speed)),
		humanDuration(eta),
	)
}

func humanBytes(b uint64) string {
	const (
		KiB = 1024
		MiB = 1024 * KiB
		GiB = 1024 * MiB
		TiB = 1024 * GiB
	)

	switch {
	case b >= TiB:
		return fmt.Sprintf("%.2fTiB", float64(b)/TiB)
	case b >= GiB:
		return fmt.Sprintf("%.2fGiB", float64(b)/GiB)
	case b >= MiB:
		return fmt.Sprintf("%.2fMiB", float64(b)/MiB)
	case b >= KiB:
		return fmt.Sprintf("%.2fKiB", float64(b)/KiB)
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func humanDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	sec := int(d.Seconds())
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	min := sec / 60
	sec = sec % 60
	if min < 60 {
		return fmt.Sprintf("%dm%ds", min, sec)
	}
	h := min / 60
	min = min % 60
	return fmt.Sprintf("%dh%dm", h, min)
}
