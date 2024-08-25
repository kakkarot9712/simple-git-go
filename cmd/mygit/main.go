package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"

	// Uncomment this block to pass the first stage!
	"os"
)

func exitIfError(err error, msg string) {
	if err != nil {
		fmt.Printf("Error: %v failed: %v", msg, err)
		os.Exit(1)
	}
}

// Usage: your_program.sh <command> <arg1> <arg2> ...
func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	// fmt.Println("Logs from your program will appear here!")

	// Uncomment this block to pass the first stage!

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: mygit <command> [<args>...]\n")
		os.Exit(1)
	}

	switch command := os.Args[1]; command {
	case "init":
		for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating directory: %s\n", err)
			}
		}

		headFileContents := []byte("ref: refs/heads/main\n")
		if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %s\n", err)
		}

		fmt.Println("Initialized git directory")

	case "cat-file":
		sha := os.Args[3]
		objectPath := ".git" + "/objects/" + sha[:2] + "/" + sha[2:]
		buff, err := os.ReadFile(objectPath)
		reader := bytes.NewReader(buff)
		if err != nil {
			fmt.Println("Error: Read failed", err)
			os.Exit(1)
		}
		r, err := zlib.NewReader(reader)
		defer r.Close()
		if err != nil {
			fmt.Println("Error: Decomp failed", err)
			os.Exit(1)
		}
		p, err := io.ReadAll(r)
		if err != nil {
			fmt.Println("Error: Zlib-Read failed", err)
			os.Exit(1)
		}
		splits := strings.Split(string(p), " ")
		if splits[0] != "blob" {
			fmt.Println("Error: Not a Blob")
			os.Exit(1)
		}
		zeroIndex := -1
		for i, b := range p {
			if b == 0 {
				zeroIndex = i
				break
			}
		}
		contentLength, err := strconv.Atoi(string(p[5:zeroIndex]))
		if err != nil {
			fmt.Println("Error: Invalid Length Info")
			os.Exit(1)
		}
		data := p[zeroIndex+1:]
		os.Stdout.Write(data[:contentLength])

	case "hash-object":
		fileName := os.Args[3]
		buff, err := os.ReadFile(fileName)
		exitIfError(err, "File Read")
		blob := []byte("blob ")
		blob = append(blob, []byte(strconv.Itoa(len(buff)))...)
		blob = append(blob, byte(0))
		blob = append(blob, buff...)
		sha := sha1.New()
		_, err = sha.Write(blob)
		exitIfError(err, "HASH Write")
		hash := sha.Sum(nil)
		hexhash := hex.EncodeToString(hash)
		exitIfError(err, "HEX DECODE")
		var in bytes.Buffer
		newFilePath := ".git/objects/" + hexhash[:2] + "/" + hexhash[2:]

		zlibWriter := zlib.NewWriter(&in)
		_, err = zlibWriter.Write(blob)
		zlibWriter.Close()
		exitIfError(err, "ZLIB Write")
		err = os.Mkdir(".git/objects/"+hexhash[:2], 0644)
		exitIfError(err, "DIR CREATE")
		err = os.WriteFile(newFilePath, in.Bytes(), 0755)
		exitIfError(err, "FILE WRITE")
		os.Stdout.Write([]byte(hexhash))

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %s\n", command)
		os.Exit(1)
	}
}
