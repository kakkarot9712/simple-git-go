package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path"
	"strconv"
)

func exitIfError(err error, msg string) {
	if err != nil {
		fmt.Printf("Error: %v failed: %v", msg, err)
		os.Exit(1)
	}
}

func getOctetFromByte(b byte) string {
	bits := fmt.Sprintf("%b", b)
	for range 8 - len(bits) {
		bits = "0" + bits
	}
	return bits
}

func hashContent(content []byte) []byte {
	sha := sha1.New()
	_, err := sha.Write(content)
	exitIfError(err, "HASH_WRITE")
	hash := sha.Sum(nil)
	return hash
}

func compressContent(content []byte) []byte {
	var in bytes.Buffer
	w := zlib.NewWriter(&in)
	_, err := w.Write(content)
	w.Close()
	exitIfError(err, "ZLIB_COMP")
	return in.Bytes()
}

func decompressContent(content []byte) ([]byte, int) {
	var buffer bytes.Buffer
	_, err := buffer.Write(content)
	exitIfError(err, "BUFF_WRITE")
	r, err := zlib.NewReader(&buffer)
	exitIfError(err, "ZLIB_READ")
	var out bytes.Buffer
	io.Copy(&out, r)
	r.Close()
	unreadBuffLen := len(buffer.Bytes())
	return out.Bytes(), unreadBuffLen
}

func getNthBitOfByte(b byte, n uint) uint {
	mask := int(math.Pow(2.0, float64(n-1)))
	if int(b)&mask == mask {
		return 1
	}
	return 0
}

func getPktLinedData(raw string) string {
	rawDataLen := len(raw)
	// 4 per length data, 1 for newLine character
	totalLen := rawDataLen + 4 + 1
	hexLen := hex.EncodeToString([]byte(string(totalLen)))
	missingBytes := 4 - len(hexLen)
	lenStr := ""
	for range missingBytes {
		lenStr += "0"
	}
	lenStr += hexLen
	return lenStr + raw + "\n"
}

func isMSB(b byte) bool {
	return getNthBitOfByte(b, 8) == 1
}

func getObjectTypeFromMSB(msb byte) Object {
	bits := getOctetFromByte(msb)
	objecType, err := strconv.ParseUint(bits[1:4], 2, 8)
	exitIfError(err, "BIN_CONV_INT")
	return Object(objecType)
}

func getLengthBitsFromByte(b byte, first bool) string {
	bits := getOctetFromByte(b)
	// 0 000 0000
	// bits := fmt.Sprintf("%b", b)
	if first {
		// X ABC PQRS
		return bits[4:]
	} else {
		// X PQRSTUV
		return bits[1:]
	}
}

func calculateLengthFromVariableBytes(data *[]byte, initCursor uint) (length uint, cursor uint, bytesConsumed uint) {
	lengthBits := ""
	cursor = initCursor
	for {
		b := (*data)[cursor]
		cursor++
		bytesConsumed++
		isMSB := isMSB(b)
		lengthBits = getLengthBitsFromByte(b, false) + lengthBits
		if !isMSB {
			converted, err := strconv.ParseInt(lengthBits, 2, 32)
			exitIfError(err, "LEN_PARSEUINT")
			length = uint(converted)
			return
		}
	}
}

func writeObjectToDisk(data []byte, hexhash string, compress bool, dest string) {
	dataToWrite := data
	if compress {
		dataToWrite = compressContent(data)
	}
	err := os.MkdirAll(path.Join(dest, ".git", "objects", hexhash[:2]), 0755)
	if err != nil && !os.IsExist(err) {
		log.Fatal("DIR_CREATE FAILED", err)
	}
	err = os.WriteFile(path.Join(dest, ".git", "objects", hexhash[:2], hexhash[2:]), dataToWrite, 0644)
	exitIfError(err, "FILE_WRITE")
}

func writeHeaderToContent(data []byte, objectType Object) []byte {
	zeroIndex := byte(0)
	lenghtBytes := []byte(strconv.Itoa(len(data)))
	contentWithHeader := []byte{}
	switch objectType {
	case Commit:
		contentWithHeader = append(contentWithHeader, []byte("commit ")...)
	case Blob:
		contentWithHeader = append(contentWithHeader, []byte("blob ")...)
	case Tree:
		contentWithHeader = append(contentWithHeader, []byte("tree ")...)
	default:
		panic("unsupported object")
	}
	contentWithHeader = append(contentWithHeader, lenghtBytes...)
	contentWithHeader = append(contentWithHeader, zeroIndex)
	contentWithHeader = append(contentWithHeader, data...)
	return contentWithHeader
}
