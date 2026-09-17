package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	"kaffeinate/internal/artwork"
)

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate() error {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		return fmt.Errorf("usage: icons output.icns [preview.png]")
	}
	var chunks bytes.Buffer
	for _, representation := range []struct {
		size int
		tag  string
	}{
		{16, "icp4"}, {32, "icp5"}, {64, "icp6"}, {128, "ic07"}, {256, "ic08"}, {512, "ic09"}, {1024, "ic10"},
	} {
		imageBytes := artwork.AppPNG(representation.size)
		chunks.WriteString(representation.tag)
		_ = binary.Write(&chunks, binary.BigEndian, uint32(len(imageBytes)+8))
		chunks.Write(imageBytes)
	}
	var iconFile bytes.Buffer
	iconFile.WriteString("icns")
	_ = binary.Write(&iconFile, binary.BigEndian, uint32(chunks.Len()+8))
	iconFile.Write(chunks.Bytes())
	if err := os.WriteFile(os.Args[1], iconFile.Bytes(), 0o644); err != nil {
		return err
	}
	if len(os.Args) == 3 {
		return os.WriteFile(os.Args[2], artwork.AppPNG(512), 0o644)
	}
	return nil
}
