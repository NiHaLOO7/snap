package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nihalkumar/snap/internal/snapshot"
	"github.com/nihalkumar/snap/internal/store"
)

var snappackMagic = []byte("SNAPPACK\x01")

func cmdExport() {
	engine := requireInit()

	if len(os.Args) < 3 {
		fatal("usage: snap export <id> [-o output] [-p password]")
	}

	id, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fatal("invalid snapshot ID: %s", os.Args[2])
	}

	var outputFile string
	var password string

	for i := 3; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "-o", "--output":
			if i+1 < len(os.Args) {
				outputFile = os.Args[i+1]
				i++
			}
		case "-p", "--password":
			if i+1 < len(os.Args) {
				password = os.Args[i+1]
				i++
			}
		}
	}

	snapshots, err := engine.List()
	if err != nil {
		fatal("list: %v", err)
	}

	var snap *snapshot.Snapshot
	for _, s := range snapshots {
		if s.ID == id {
			snap = s
			break
		}
	}
	if snap == nil {
		fatal("snapshot #%d not found", id)
	}

	if outputFile == "" {
		safeName := strings.ReplaceAll(snap.Message, " ", "-")
		safeName = strings.ReplaceAll(safeName, "/", "_")
		if len(safeName) > 40 {
			safeName = safeName[:40]
		}
		outputFile = fmt.Sprintf("checkpoint-%d-%s.snap", id, safeName)
	}

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")
	objStore := store.New(snapPath)

	meta := map[string]interface{}{
		"id":          snap.ID,
		"message":     snap.Message,
		"description": snap.Description,
		"timestamp":   snap.Timestamp.Format(time.RFC3339),
		"file_count":  snap.FileCount,
		"tree":        snap.Tree,
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		fatal("marshal metadata: %v", err)
	}

	uniqueHashes := make(map[string]bool)
	for _, hash := range snap.Tree {
		uniqueHashes[hash] = true
	}

	var payload []byte

	metaLen := make([]byte, 4)
	binary.BigEndian.PutUint32(metaLen, uint32(len(metaJSON)))
	payload = append(payload, metaLen...)
	payload = append(payload, metaJSON...)

	blobCount := make([]byte, 4)
	binary.BigEndian.PutUint32(blobCount, uint32(len(uniqueHashes)))
	payload = append(payload, blobCount...)

	for hash := range uniqueHashes {
		data, err := objStore.ReadRaw(hash)
		if err != nil {
			fatal("read blob %s: %v", hash[:8], err)
		}

		payload = append(payload, []byte(hash)...)

		dataLen := make([]byte, 4)
		binary.BigEndian.PutUint32(dataLen, uint32(len(data)))
		payload = append(payload, dataLen...)
		payload = append(payload, data...)
	}

	checksum := sha256.Sum256(payload)
	payload = append(payload, checksum[:]...)

	var output []byte
	flags := byte(0)
	if password != "" {
		flags = 1
	}
	output = append(output, snappackMagic...)
	output = append(output, flags)

	if password != "" {
		key := sha256.Sum256([]byte(password))
		block, err := aes.NewCipher(key[:])
		if err != nil {
			fatal("create cipher: %v", err)
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			fatal("create GCM: %v", err)
		}
		nonce := make([]byte, gcm.NonceSize())
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			fatal("generate nonce: %v", err)
		}
		encrypted := gcm.Seal(nonce, nonce, payload, nil)
		output = append(output, encrypted...)
	} else {
		output = append(output, payload...)
	}

	if err := os.WriteFile(outputFile, output, 0644); err != nil {
		fatal("write file: %v", err)
	}

	fmt.Printf("Exported snapshot #%d → %s\n", id, outputFile)
	fmt.Printf("  Message:  %s\n", snap.Message)
	fmt.Printf("  Files:    %d\n", snap.FileCount)
	fmt.Printf("  Size:     %s\n", formatSize(int64(len(output))))
	if password != "" {
		fmt.Printf("  Password: protected ✓\n")
	}
	fmt.Printf("\nShare this file. Import with: snap import %s\n", outputFile)
}

func cmdImport() {
	engine := requireInit()

	if len(os.Args) < 3 {
		fatal("usage: snap import <file.snap> [-p password]")
	}

	inputFile := os.Args[2]
	var password string

	for i := 3; i < len(os.Args); i++ {
		if (os.Args[i] == "-p" || os.Args[i] == "--password") && i+1 < len(os.Args) {
			password = os.Args[i+1]
			i++
		}
	}

	data, err := os.ReadFile(inputFile)
	if err != nil {
		fatal("read file: %v", err)
	}

	if len(data) < 10 || string(data[:9]) != string(snappackMagic) {
		fatal("not a valid .snap file (bad magic)")
	}

	flags := data[9]
	payload := data[10:]

	if flags&1 != 0 {
		if password == "" {
			fatal("this file is password-protected. Use: snap import %s -p \"password\"", inputFile)
		}
		key := sha256.Sum256([]byte(password))
		block, err := aes.NewCipher(key[:])
		if err != nil {
			fatal("create cipher: %v", err)
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			fatal("create GCM: %v", err)
		}
		nonceSize := gcm.NonceSize()
		if len(payload) < nonceSize {
			fatal("corrupted file (too short)")
		}
		nonce, ciphertext := payload[:nonceSize], payload[nonceSize:]
		payload, err = gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			fatal("wrong password or corrupted file")
		}
	}

	if len(payload) < 32 {
		fatal("corrupted file (no checksum)")
	}
	content := payload[:len(payload)-32]
	expectedChecksum := payload[len(payload)-32:]
	actualChecksum := sha256.Sum256(content)
	if string(actualChecksum[:]) != string(expectedChecksum) {
		fatal("corrupted file (checksum mismatch)")
	}

	offset := 0

	if offset+4 > len(content) {
		fatal("corrupted file (no metadata length)")
	}
	metaLen := binary.BigEndian.Uint32(content[offset : offset+4])
	offset += 4

	if offset+int(metaLen) > len(content) {
		fatal("corrupted file (metadata truncated)")
	}
	metaJSON := content[offset : offset+int(metaLen)]
	offset += int(metaLen)

	var meta map[string]interface{}
	if err := json.Unmarshal(metaJSON, &meta); err != nil {
		fatal("parse metadata: %v", err)
	}

	if offset+4 > len(content) {
		fatal("corrupted file (no blob count)")
	}
	blobCountVal := binary.BigEndian.Uint32(content[offset : offset+4])
	offset += 4

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")
	objStore := store.New(snapPath)

	for i := 0; i < int(blobCountVal); i++ {
		if offset+64 > len(content) {
			fatal("corrupted file (blob %d hash truncated)", i)
		}
		hash := string(content[offset : offset+64])
		offset += 64

		if offset+4 > len(content) {
			fatal("corrupted file (blob %d length truncated)", i)
		}
		dataLen := binary.BigEndian.Uint32(content[offset : offset+4])
		offset += 4

		if offset+int(dataLen) > len(content) {
			fatal("corrupted file (blob %d data truncated)", i)
		}
		blobData := content[offset : offset+int(dataLen)]
		offset += int(dataLen)

		if !objStore.Has(hash) {
			if err := objStore.WriteRaw(hash, blobData); err != nil {
				fatal("write blob %s: %v", hash[:8], err)
			}
		}
	}

	tree := make(map[string]string)
	if treeData, ok := meta["tree"].(map[string]interface{}); ok {
		for path, hash := range treeData {
			tree[path] = hash.(string)
		}
	}

	message := "imported"
	if msg, ok := meta["message"].(string); ok {
		message = fmt.Sprintf("[imported] %s", msg)
	}
	description := ""
	if desc, ok := meta["description"].(string); ok {
		description = desc
	}

	snap, err := engine.SaveFromTree(tree, message, description)
	if err != nil {
		fatal("create snapshot: %v", err)
	}

	fmt.Printf("Imported → Snapshot #%d\n", snap.ID)
	fmt.Printf("  Message:  %s\n", message)
	fmt.Printf("  Files:    %d\n", len(tree))
	fmt.Printf("  Blobs:    %d imported\n", blobCountVal)
	if desc, ok := meta["description"].(string); ok && desc != "" {
		fmt.Printf("  Desc:     %s\n", desc)
	}
}
