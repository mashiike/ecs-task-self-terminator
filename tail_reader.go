package main

import (
	"context"
	"io"
	"os"
	"sync"
	"time"
)

type TailReader struct {
	mu         sync.Mutex
	fileName   string
	currentPos int64
	ctx        context.Context
}

func NewTailReader(fileName string) (*TailReader, error) {
	return NewTailReaderWithContext(context.Background(), fileName)
}

func NewTailReaderWithContext(ctx context.Context, fileName string) (*TailReader, error) {
	_, err := os.Stat(fileName)
	if err != nil {
		return nil, err
	}
	r := &TailReader{
		fileName: fileName,
		ctx:      ctx,
	}
	return r.WithContext(ctx), nil
}

func (r *TailReader) WithContext(ctx context.Context) *TailReader {
	cloned := &TailReader{
		fileName:   r.fileName,
		currentPos: r.currentPos,
	}
	cloned.ctx = ctx
	return cloned
}

func (r *TailReader) Context() context.Context {
	return r.ctx
}

func (r *TailReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		select {
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		default:
		}
		stats, err := os.Stat(r.fileName)
		if err != nil {
			return 0, err
		}
		if stats.Size() <= r.currentPos {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		file, err := os.Open(r.fileName)
		if err != nil {
			return 0, err
		}
		_, err = file.Seek(r.currentPos, io.SeekStart)
		if err != nil {
			return 0, err
		}
		n, err := file.Read(p)
		if err != nil {
			return 0, err
		}
		if err := file.Close(); err != nil {
			return 0, err
		}
		r.currentPos += int64(n)
		return n, nil
	}
}
