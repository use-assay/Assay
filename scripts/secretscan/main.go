package main

import (
	"bufio"
	"encoding/base32"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	seedVersionByte = 18 << 3
	allowMarker     = "secret-scan: allow-seed"
)

var seedCandidate = regexp.MustCompile(`S[A-Z2-7]{55}`)

type Finding struct {
	File string
	Line int
	Seed string
}

func main() {
	diff := flag.Bool("diff", false, "read a unified git diff from stdin")
	flag.Parse()

	var (
		findings []Finding
		err      error
	)
	if *diff {
		findings, err = ScanDiff(os.Stdin)
	} else {
		findings, err = ScanFiles(flag.Args())
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "secret-scan: %v\n", err)
		os.Exit(2)
	}
	for _, f := range findings {
		fmt.Printf("%s:%d: Stellar secret seed (valid strkey checksum)\n", f.File, f.Line)
	}
	if len(findings) > 0 {
		os.Exit(1)
	}
}

func ScanFiles(paths []string) ([]Finding, error) {
	if len(paths) == 0 {
		return ScanText(os.Stdin, "<stdin>")
	}
	var findings []Finding
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", path, err)
		}
		found, err := ScanText(f, path)
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", path, closeErr)
		}
		if err != nil {
			return nil, err
		}
		findings = append(findings, found...)
	}
	return findings, nil
}

func ScanText(r io.Reader, name string) ([]Finding, error) {
	var findings []Finding
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		if f, ok := scanLine(scanner.Text(), name, line); ok {
			findings = append(findings, f)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", name, err)
	}
	return findings, nil
}

func ScanDiff(r io.Reader) ([]Finding, error) {
	var findings []Finding
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	file := "<diff>"
	newLine := 0
	inHunk := false
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inHunk = false
		case inHunk && strings.HasPrefix(line, "+"):
			if f, ok := scanLine(strings.TrimPrefix(line, "+"), file, newLine); ok {
				findings = append(findings, f)
			}
			newLine++
		case inHunk && strings.HasPrefix(line, " "):
			newLine++
		case inHunk && strings.HasPrefix(line, "-"):
		case inHunk && strings.HasPrefix(line, `\`):
		case !inHunk && strings.HasPrefix(line, "+++ "):
			file = strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
		case !inHunk && strings.HasPrefix(line, "@@ "):
			newLine = hunkStart(line)
			inHunk = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan diff: %w", err)
	}
	return findings, nil
}

func hunkStart(header string) int {
	plus := strings.Index(header, " +")
	if plus < 0 {
		return 0
	}
	rest := header[plus+2:]
	end := strings.IndexAny(rest, ", ")
	if end < 0 {
		end = len(rest)
	}
	start, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0
	}
	return start
}

func scanLine(line, file string, number int) (Finding, bool) {
	if strings.Contains(line, allowMarker) {
		return Finding{}, false
	}
	for _, loc := range seedCandidate.FindAllStringIndex(line, -1) {
		start, end := loc[0], loc[1]
		if start > 0 && isWordByte(line[start-1]) {
			continue
		}
		if end < len(line) && isWordByte(line[end]) {
			continue
		}
		seed := line[start:end]
		if validSeed(seed) {
			return Finding{File: file, Line: number, Seed: seed}, true
		}
	}
	return Finding{}, false
}

func isWordByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func validSeed(seed string) bool {
	if len(seed) != 56 || seed[0] != 'S' {
		return false
	}
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(seed)
	if err != nil || len(raw) != 35 || raw[0] != seedVersionByte {
		return false
	}
	sum := checksum(raw[:33])
	return sum[0] == raw[33] && sum[1] == raw[34]
}

func checksum(data []byte) [2]byte {
	crc := uint16(0)
	for _, b := range data {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return [2]byte{byte(crc), byte(crc >> 8)}
}
