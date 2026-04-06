package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ErmilovAlexander/imgconv/internal/formats/raw"
)

func BenchmarkConvertRangeRawToRaw(b *testing.B) {
	dir := b.TempDir()
	rawPath := filepath.Join(dir, "src.raw")
	srcData := make([]byte, 4<<20)
	copy(srcData[0:16], []byte("bench-raw-0000001"))
	copy(srcData[2<<20:2<<20+16], []byte("bench-raw-0000002"))
	if err := os.WriteFile(rawPath, srcData, 0o644); err != nil {
		b.Fatalf("write raw: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in, err := raw.Open(rawPath)
		if err != nil {
			b.Fatalf("open raw: %v", err)
		}
		outPath := filepath.Join(dir, "out-"+itoa(i)+".raw")
		err = ConvertRange(context.Background(), in, outPath, ConvertRangeOptions{
			Threads:   2,
			Sparse:    true,
			ChunkSize: 64 << 10,
			Format:    "raw",
		})
		_ = in.Close()
		if err != nil {
			b.Fatalf("convert: %v", err)
		}
		_ = os.Remove(outPath)
	}
}

func BenchmarkConvertRangeRawToQCOW2(b *testing.B) {
	dir := b.TempDir()
	rawPath := filepath.Join(dir, "src.raw")
	srcData := make([]byte, 4<<20)
	copy(srcData[0:16], []byte("bench-qcow2-00001"))
	copy(srcData[3<<20:3<<20+16], []byte("bench-qcow2-00002"))
	if err := os.WriteFile(rawPath, srcData, 0o644); err != nil {
		b.Fatalf("write raw: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in, err := raw.Open(rawPath)
		if err != nil {
			b.Fatalf("open raw: %v", err)
		}
		outPath := filepath.Join(dir, "out-"+itoa(i)+".qcow2")
		err = ConvertRange(context.Background(), in, outPath, ConvertRangeOptions{
			Threads:     2,
			Sparse:      true,
			ChunkSize:   64 << 10,
			Format:      "qcow2",
			ClusterBits: 16,
		})
		_ = in.Close()
		if err != nil {
			b.Fatalf("convert: %v", err)
		}
		_ = os.Remove(outPath)
	}
}

