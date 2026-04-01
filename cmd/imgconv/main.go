package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"imgconv/internal/formats/qcow2"
	"imgconv/internal/formats/vdi"
	"imgconv/internal/formats/vmdk"
	"imgconv/internal/image"
	"imgconv/internal/ops"
	"imgconv/internal/pipeline"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "info":
		cmdInfo(os.Args[2:])
	case "convert":
		cmdConvert(os.Args[2:])
	case "check":
		cmdCheck(os.Args[2:])
	case "create":
		cmdCreate(os.Args[2:])
	case "compare":
		cmdCompare(os.Args[2:])
	case "commit":
		cmdCommit(os.Args[2:])
	case "rebase":
		cmdRebase(os.Args[2:])
	case "map":
		cmdMap(os.Args[2:])
	case "measure":
		cmdMeasure(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Print(`imgconv

Usage:
  imgconv info -i <image> [--input-format raw|qcow2|vmdk|vdi] [--json] [--debug]
  imgconv convert -i <input> -o <output> [--input-format raw|qcow2|vmdk|vdi] --format raw|qcow2|vdi|vmdk [--sparse] [--threads N] [--chunk-mib N] [--verify none|sample|full] [--debug]
  imgconv check -i <image> [--input-format qcow2|vmdk|vdi] [--debug]
  imgconv create -f raw|qcow2|vdi|vmdk -o <output> --size <SIZE> [--sparse] [--cluster-bits N] [--block-size N] [--backing-file PATH]
  imgconv compare -a <imageA> -b <imageB> [--input-format-a raw|qcow2|vmdk|vdi] [--input-format-b raw|qcow2|vmdk|vdi] [--mode none|sample|full] [--chunk-mib N]
  imgconv commit -i <overlay.qcow2> [--chunk-mib N]
  imgconv rebase -i <overlay.qcow2> --backing-file <PATH>
  imgconv map -i <qcow2> [--json]
  imgconv measure -f qcow2 --size <SIZE> [--cluster-bits N] [--backing-file PATH] [--json]

Examples:
  imgconv create -f qcow2 -o base.qcow2 --size 64G
  imgconv create -f qcow2 -o overlay.qcow2 --size 64G --backing-file base.qcow2
  imgconv create -f vmdk -o disk.vmdk --size 64G
  imgconv convert -i overlay.qcow2 -o flat.qcow2 --format qcow2 --verify full
  imgconv convert -i src.qcow2 -o out.vmdk --format vmdk --verify full
  imgconv compare -a src.vmdk -b out.qcow2 --mode full
  imgconv commit -i overlay.qcow2
  imgconv rebase -i overlay.qcow2 --backing-file newbase.qcow2
  imgconv map -i overlay.qcow2 --json
  imgconv measure -f qcow2 --size 500G --cluster-bits 16 --json
`)
}

func cmdInfo(args []string) {
	fs := flag.NewFlagSet("info", flag.ExitOnError)

	inPath := fs.String("i", "", "input image path")
	inFmt := fs.String("input-format", "", "input format")
	asJSON := fs.Bool("json", false, "print JSON")
	debug := fs.Bool("debug", false, "enable VMDK debug logging")

	_ = fs.Parse(args)

	if *inPath == "" {
		fmt.Fprintln(os.Stderr, "info: -i is required")
		os.Exit(2)
	}

	vmdk.SetDebug(*debug)

	info, err := image.Inspect(*inPath, *inFmt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "info failed:", err)
		os.Exit(1)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(info)
		return
	}

	fmt.Printf("Path: %s\n", info.Path)
	fmt.Printf("Format: %s\n", info.Format)
	fmt.Printf("Virtual size: %d bytes\n", info.VirtualSize)
	fmt.Printf("File size: %d bytes\n", info.FileSize)
	for k, v := range info.Details {
		fmt.Printf("%s: %v\n", k, v)
	}
}

func cmdConvert(args []string) {
	fs := flag.NewFlagSet("convert", flag.ExitOnError)

	inPath := fs.String("i", "", "input path")
	outPath := fs.String("o", "", "output path")
	inFmt := fs.String("input-format", "", "input format")
	outFmt := fs.String("format", "raw", "output format: raw|qcow2|vdi|vmdk")
	sparse := fs.Bool("sparse", false, "sparse output")
	threads := fs.Int("threads", runtime.NumCPU(), "worker threads")
	verify := fs.String("verify", "sample", "verify mode: none|sample|full")
	chunkMiB := fs.Int("chunk-mib", 4, "chunk size in MiB")
	debug := fs.Bool("debug", false, "enable VMDK debug logging")

	_ = fs.Parse(args)

	vmdk.SetDebug(*debug)

	if *inPath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "convert: -i and -o are required")
		os.Exit(2)
	}
	if *outFmt != "raw" && *outFmt != "qcow2" && *outFmt != "vdi" && *outFmt != "vmdk" {
		fmt.Fprintln(os.Stderr, "convert: --format must be raw or qcow2 or vdi or vmdk")
		os.Exit(2)
	}

	vm := pipeline.VerifyMode(*verify)
	switch vm {
	case pipeline.VerifyNone, pipeline.VerifySample, pipeline.VerifyFull:
	default:
		fmt.Fprintln(os.Stderr, "convert: invalid --verify, use none|sample|full")
		os.Exit(2)
	}

	chunkSize := uint64(*chunkMiB) << 20
	if chunkSize == 0 {
		chunkSize = 4 << 20
	}

	src, err := image.Open(*inPath, *inFmt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open input failed:", err)
		os.Exit(1)
	}
	defer src.Reader.Close()

	reopen := func() (pipeline.RangeReader, error) {
		res, err := image.Open(*inPath, *inFmt)
		if err != nil {
			return nil, err
		}
		return res.Reader, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	start := time.Now()

	if err := pipeline.ConvertRange(ctx, src.Reader, *outPath, pipeline.ConvertRangeOptions{
		Threads:        *threads,
		Sparse:         *sparse,
		ChunkSize:      chunkSize,
		ProgressWriter: os.Stderr,
		Format:         *outFmt,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "convert failed:", err)
		os.Exit(1)
	}

	if vm != pipeline.VerifyNone {
		if err := pipeline.VerifyRange(ctx, reopen, *outPath, *outFmt, pipeline.VerifyOptions{
			Mode:         vm,
			SampleBlocks: 256,
			ChunkSize:    chunkSize,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "verify failed:", err)
			os.Exit(1)
		}
	}

	fmt.Fprintf(os.Stderr, "convert OK in %s\n", time.Since(start).Truncate(10*time.Millisecond))
}

func cmdCheck(args []string) {
	fs := flag.NewFlagSet("check", flag.ExitOnError)

	inPath := fs.String("i", "", "input image path")
	inFmt := fs.String("input-format", "", "input format")
	debug := fs.Bool("debug", false, "enable VMDK debug logging")

	_ = fs.Parse(args)

	if *inPath == "" {
		fmt.Fprintln(os.Stderr, "check: -i is required")
		os.Exit(2)
	}

	vmdk.SetDebug(*debug)

	fmtHint, err := image.ParseFormat(*inFmt)
	if err != nil {
		fmt.Fprintln(os.Stderr, "check:", err)
		os.Exit(2)
	}
	if fmtHint == "" {
		fmtHint = image.DetectFormat(*inPath)
	}

	switch fmtHint {
	case image.FormatQCOW2:
		if err := pipeline.CheckQCOW2(*inPath); err != nil {
			fmt.Fprintln(os.Stderr, "check failed:", err)
			os.Exit(1)
		}
		fmt.Println("qcow2 check: OK")

	case image.FormatVMDK:
		r, err := vmdk.Open(*inPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "check failed:", err)
			os.Exit(1)
		}
		defer r.Close()
		fmt.Printf("vmdk check: OK\nVirtual size: %d bytes\n", r.Size())

	case image.FormatVDI:
		if err := pipeline.CheckVDI(*inPath); err != nil {
			fmt.Fprintln(os.Stderr, "check failed:", err)
			os.Exit(1)
		}
		r, err := vdi.Open(*inPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "check failed:", err)
			os.Exit(1)
		}
		defer r.Close()
		fmt.Printf("vdi check: OK\nVirtual size: %d bytes\n", r.Size())

	default:
		fmt.Fprintln(os.Stderr, "check: supported formats are qcow2, vmdk and vdi")
		os.Exit(2)
	}
}

func cmdCreate(args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)

	format := fs.String("f", "", "output format: raw|qcow2|vdi|vmdk")
	outPath := fs.String("o", "", "output path")
	sizeStr := fs.String("size", "", "virtual size, e.g. 64G, 500M, 1048576")
	sparse := fs.Bool("sparse", true, "create sparse image where supported")
	clusterBits := fs.Uint("cluster-bits", 16, "qcow2 cluster bits")
	blockSize := fs.Uint("block-size", 1<<20, "vdi block size in bytes")
	backingFile := fs.String("backing-file", "", "qcow2 backing file path")

	_ = fs.Parse(args)

	if *format == "" || *outPath == "" || *sizeStr == "" {
		fmt.Fprintln(os.Stderr, "create: -f, -o and --size are required")
		os.Exit(2)
	}

	f, err := image.ParseFormat(*format)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create:", err)
		os.Exit(2)
	}
	if f != image.FormatRAW && f != image.FormatQCOW2 && f != image.FormatVDI && f != image.FormatVMDK {
		fmt.Fprintln(os.Stderr, "create: only raw, qcow2, vdi and vmdk are supported")
		os.Exit(2)
	}

	size, err := parseSize(*sizeStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create:", err)
		os.Exit(2)
	}

	w, err := image.Create(*outPath, f, image.CreateOptions{
		Size:        size,
		Sparse:      *sparse,
		ClusterBits: uint32(*clusterBits),
		BlockSize:   uint32(*blockSize),
		BackingFile: *backingFile,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "create failed:", err)
		os.Exit(1)
	}
	if err := w.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "create close failed:", err)
		os.Exit(1)
	}

	fmt.Printf("created %s format=%s size=%d bytes\n", *outPath, f, size)
}

func cmdCompare(args []string) {
	fs := flag.NewFlagSet("compare", flag.ExitOnError)

	aPath := fs.String("a", "", "image A")
	bPath := fs.String("b", "", "image B")
	aFmt := fs.String("input-format-a", "", "format of image A")
	bFmt := fs.String("input-format-b", "", "format of image B")
	mode := fs.String("mode", "full", "compare mode: none|sample|full")
	chunkMiB := fs.Int("chunk-mib", 4, "chunk size in MiB")

	_ = fs.Parse(args)

	if *aPath == "" || *bPath == "" {
		fmt.Fprintln(os.Stderr, "compare: -a and -b are required")
		os.Exit(2)
	}

	vm := pipeline.VerifyMode(*mode)
	switch vm {
	case pipeline.VerifyNone, pipeline.VerifySample, pipeline.VerifyFull:
	default:
		fmt.Fprintln(os.Stderr, "compare: invalid --mode, use none|sample|full")
		os.Exit(2)
	}

	chunkSize := uint64(*chunkMiB) << 20
	if chunkSize == 0 {
		chunkSize = 4 << 20
	}

	if err := ops.ComparePaths(context.Background(), *aPath, *aFmt, *bPath, *bFmt, ops.CompareOptions{
		Mode:         vm,
		SampleBlocks: 256,
		ChunkSize:    chunkSize,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "compare failed:", err)
		os.Exit(1)
	}

	fmt.Println("compare: OK")
}

func cmdCommit(args []string) {
	fs := flag.NewFlagSet("commit", flag.ExitOnError)

	inPath := fs.String("i", "", "overlay qcow2 path")
	chunkMiB := fs.Int("chunk-mib", 4, "chunk size in MiB")

	_ = fs.Parse(args)

	if *inPath == "" {
		fmt.Fprintln(os.Stderr, "commit: -i is required")
		os.Exit(2)
	}

	chunkSize := uint64(*chunkMiB) << 20
	if chunkSize == 0 {
		chunkSize = 4 << 20
	}

	if err := ops.CommitQCOW2Overlay(context.Background(), *inPath, ops.CommitOptions{
		ChunkSize: chunkSize,
		Sparse:    true,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "commit failed:", err)
		os.Exit(1)
	}

	fmt.Println("commit: OK")
}

func cmdRebase(args []string) {
	fs := flag.NewFlagSet("rebase", flag.ExitOnError)

	inPath := fs.String("i", "", "overlay qcow2 path")
	backingFile := fs.String("backing-file", "", "new backing file path (empty string clears backing)")

	_ = fs.Parse(args)

	if *inPath == "" {
		fmt.Fprintln(os.Stderr, "rebase: -i is required")
		os.Exit(2)
	}

	if err := qcow2.RebasePath(*inPath, *backingFile); err != nil {
		fmt.Fprintln(os.Stderr, "rebase failed:", err)
		os.Exit(1)
	}

	fmt.Println("rebase: OK")
}

func cmdMap(args []string) {
	fs := flag.NewFlagSet("map", flag.ExitOnError)

	inPath := fs.String("i", "", "qcow2 image path")
	asJSON := fs.Bool("json", false, "print JSON")

	_ = fs.Parse(args)

	if *inPath == "" {
		fmt.Fprintln(os.Stderr, "map: -i is required")
		os.Exit(2)
	}

	exts, err := qcow2.MapFile(*inPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "map failed:", err)
		os.Exit(1)
	}

	if *asJSON {
		if err := qcow2.WriteMapJSON(os.Stdout, exts); err != nil {
			fmt.Fprintln(os.Stderr, "map output failed:", err)
			os.Exit(1)
		}
		return
	}

	if err := qcow2.WriteMapText(os.Stdout, exts); err != nil {
		fmt.Fprintln(os.Stderr, "map output failed:", err)
		os.Exit(1)
	}
}

func cmdMeasure(args []string) {
	fs := flag.NewFlagSet("measure", flag.ExitOnError)

	format := fs.String("f", "", "format to measure (currently only qcow2)")
	sizeStr := fs.String("size", "", "virtual size, e.g. 64G")
	clusterBits := fs.Uint("cluster-bits", 16, "qcow2 cluster bits")
	backingFile := fs.String("backing-file", "", "optional backing file path")
	asJSON := fs.Bool("json", false, "print JSON")

	_ = fs.Parse(args)

	if *format != "qcow2" {
		fmt.Fprintln(os.Stderr, "measure: only qcow2 is supported")
		os.Exit(2)
	}
	if *sizeStr == "" {
		fmt.Fprintln(os.Stderr, "measure: --size is required")
		os.Exit(2)
	}

	size, err := parseSize(*sizeStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "measure:", err)
		os.Exit(2)
	}

	res, err := qcow2.Measure(qcow2.MeasureOptions{
		Size:        size,
		ClusterBits: uint32(*clusterBits),
		BackingFile: *backingFile,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "measure failed:", err)
		os.Exit(1)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}

	fmt.Printf("format: %s\n", res.Format)
	fmt.Printf("virtual_size: %d\n", res.VirtualSize)
	fmt.Printf("cluster_bits: %d\n", res.ClusterBits)
	fmt.Printf("cluster_size: %d\n", res.ClusterSize)
	fmt.Printf("l1_entries: %d\n", res.L1Entries)
	fmt.Printf("l1_clusters: %d\n", res.L1Clusters)
	fmt.Printf("max_data_clusters: %d\n", res.MaxDataClusters)
	fmt.Printf("max_l2_clusters: %d\n", res.MaxL2Clusters)
	fmt.Printf("refcount_block_entries: %d\n", res.RefcountBlockEntries)
	fmt.Printf("refcount_block_count: %d\n", res.RefcountBlockCount)
	fmt.Printf("refcount_table_clusters: %d\n", res.RefcountTableClusters)
	fmt.Printf("metadata_clusters: %d\n", res.MetadataClusters)
	fmt.Printf("metadata_bytes: %d\n", res.MetadataBytes)
	if res.BackingFile != "" {
		fmt.Printf("backing_file: %s\n", res.BackingFile)
	}
}

func parseSize(s string) (uint64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}

	mult := uint64(1)
	switch {
	case strings.HasSuffix(s, "K"):
		mult = 1 << 10
		s = strings.TrimSuffix(s, "K")
	case strings.HasSuffix(s, "M"):
		mult = 1 << 20
		s = strings.TrimSuffix(s, "M")
	case strings.HasSuffix(s, "G"):
		mult = 1 << 30
		s = strings.TrimSuffix(s, "G")
	case strings.HasSuffix(s, "T"):
		mult = 1 << 40
		s = strings.TrimSuffix(s, "T")
	}

	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad size %q", s)
	}
	return v * mult, nil
}
