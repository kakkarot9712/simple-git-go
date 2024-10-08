package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
)

type ObjectPerm string

const (
	FILE    ObjectPerm = "100644"
	EXE     ObjectPerm = "100755"
	SYMLINK ObjectPerm = "120000"
	DIR     ObjectPerm = "40000"
)

type tree struct {
	perm ObjectPerm
	name string
	sha  [20]byte
}

type Object uint

const (
	Unsepcified Object = iota
	Commit
	Tree
	Blob
	Tag
	_
	OFSDelta
	REFDelta
)

type status uint8

const (
	HeaderProcessingStart status = iota
	HeaderProcessing
	UndeltifiedObjectExtractionStarts
	DeltifiedObjBasePtrExtractionStarts
)

type ofsRefObject struct {
	object             []byte
	baseObjectIndex    int
	currentObjectIndex int
}

type gitObject struct {
	objectType Object
	content    []byte
}

type objectRef struct {
	Hash            string
	ObjectType      Object
	BaseObjectIndex int
}

func createBlobObject(data []byte) string {
	blob := writeHeaderToContent(data, Blob)
	hash := hashContent(blob)
	commitHex := hex.EncodeToString(hash)
	writeObjectToDisk(blob, commitHex, true, ".")
	return commitHex
}

func createCommitObject(data []byte) string {
	commit := writeHeaderToContent(data, Commit)
	commitHash := hashContent([]byte(commit))
	commitHex := hex.EncodeToString(commitHash)
	writeObjectToDisk([]byte(commit), commitHex, true, ".")
	return commitHex
}

func createTreeObject(dirpath string) (hash []byte) {
	files, err := os.ReadDir(dirpath)
	exitIfError(err, "READ_DIR")
	// fmt.Println(files)
	content := ""
	zeroByte := byte(0)
	for _, file := range files {
		if file.Name() != ".git" {
			if file.IsDir() {
				// fmt.Println(path.Join(dirpath, file.Name()) + "_DIR")
				hash := createTreeObject(path.Join(dirpath, file.Name()))
				content += string(DIR) + " " + file.Name() + string(zeroByte) + string(hash)
				// Calc Hash Rec
			} else {
				// fmt.Println(file.Name() + "_FILE")
				buff, err := os.ReadFile(path.Join(dirpath, file.Name()))
				exitIfError(err, "File Read")
				hexhash := createBlobObject(buff)
				hash, err := hex.DecodeString(hexhash)
				exitIfError(err, "HEXTOHASH CONV")
				content += string(FILE) + " " + file.Name() + string(zeroByte) + string(hash)
				// fmt.Println(content, "C")
			}
		}
	}
	treeContent := writeHeaderToContent([]byte(content), Tree)
	hash = hashContent(treeContent)
	hexHash := hex.EncodeToString(hash)
	writeObjectToDisk(treeContent, hexHash, true, ".")
	return
}

func discoverRefs(repoUrl string) string {
	getRefUrl := repoUrl + "/info/refs?service=git-upload-pack"
	resp, err := http.Get(getRefUrl)
	exitIfError(err, "REF_GET_FAILED")
	var out bytes.Buffer
	io.Copy(&out, resp.Body)
	chunks := strings.Split(out.String(), "\n")
	resId := strings.Split(chunks[0], " ")[0]
	resService := strings.Split(chunks[0], " ")[1]
	regexRes, err := regexp.MatchString("^[0-9a-f]{4}#", resId)
	exitIfError(err, "REGEX_VERIFY_FAILED")
	if chunks[len(chunks)-1] != "0000" {
		log.Fatal("invalid response end received! aborting...")
	}
	if !regexRes {
		log.Fatal("response validation failed! aborting...")
	}
	if resService != "service=git-upload-pack" {
		log.Fatal("unsupported service received! aborting...")
	}
	return out.String()
}

func getDefaultBranchFromRefs(ref string) (string, string) {
	chunks := strings.Split(ref, "\n")
	metaChunks := strings.Split(chunks[1], " ")
	symRef := ""
	for _, m := range metaChunks {
		if strings.HasPrefix(m, "symref") {
			symRef = strings.Split(m, "=")[1]
			break
		}
	}
	var defBranchSha string
	// var defBrancName string
	for _, chunk := range chunks {
		if strings.HasSuffix(chunk, strings.Split(symRef, ":")[1]) {
			// first 4 bytes are length in hex
			defBranchSha = chunk[4:44]
			// defBrancName = chunk[45:]
			break
		}
	}
	return defBranchSha, strings.Split(symRef, ":")[1]
}

func getPackDataFromBranchSha(repoUrl string, defBranchSha string) string {
	getPackUrl := repoUrl + "/git-upload-pack"
	ctypeHeader := "application/x-git-upload-pack-request"
	// side-band-64k --> For Extra information
	wantStrWithNeg := "want " + defBranchSha + " multi_ack ofs-delta"
	wantStr := "want " + defBranchSha
	bodyStr := getPktLinedData(wantStrWithNeg) + getPktLinedData(wantStr) + "0000" + getPktLinedData("done")
	body := bytes.NewReader([]byte(bodyStr))
	resp, err := http.Post(getPackUrl, ctypeHeader, body)
	exitIfError(err, "get pack failed!")
	var data bytes.Buffer
	io.Copy(&data, resp.Body)
	if string(data.String()[:7]) != "0008NAK" {
		log.Fatal("invalid response start detected")
	}
	return data.String()[8:]
}

func getPackFileMetadata(packfile string) (version uint32, objectsLength uint32) {
	packHeader := packfile[4:12]
	versonBytes := packHeader[0:4]
	objectAmountBytes := packHeader[4:8]
	objectsLength = binary.BigEndian.Uint32([]byte(objectAmountBytes))
	version = binary.BigEndian.Uint32([]byte(versonBytes))
	return
}

func decodeBlobObject(blob []byte, compressed bool) []byte {
	rawData := blob
	if compressed {
		rawData, _ = decompressContent(blob)
	}
	splits := strings.Split(string(rawData), " ")
	if splits[0] != "blob" {
		fmt.Println("Error: Not a Blob")
		os.Exit(1)
	}
	zeroIndex := -1
	for i, b := range rawData {
		if b == 0 {
			zeroIndex = i
			break
		}
	}
	contentLength, err := strconv.Atoi(string(rawData[5:zeroIndex]))
	if err != nil {
		fmt.Println("Error: Invalid Length Info")
		os.Exit(1)
	}
	data := rawData[zeroIndex+1:]
	return data[:contentLength]
}

func decodeTreeObject(rawTree []byte, compressed bool) []tree {
	out := rawTree
	if compressed {
		// Decompress to raw using zlib
		out, _ = decompressContent(rawTree)
	}
	if !bytes.HasPrefix(out, []byte("tree ")) {
		fmt.Println("Error: Not tree!")
		os.Exit(1)
	}
	zeroByteIndex := bytes.Index(out[5:], []byte{0})
	// size, err := strconv.Atoi(strings.Split(string(out[5:zeroByteIndex]), " ")[1])
	// exitIfError(err, "INVALID_SIZE")
	cursor := 5 + zeroByteIndex + 1
	ftree := tree{}
	var trees []tree
	for {
		spIndex := bytes.Index(out[cursor:], []byte(" "))
		zeroIndex := bytes.Index(out[cursor:], []byte{0})
		ftree.perm = ObjectPerm(out[cursor : cursor+spIndex])
		ftree.name = string(out[cursor+spIndex+1 : cursor+zeroIndex])
		ftree.sha = [20]byte(out[cursor+zeroIndex+1 : cursor+zeroIndex+1+20])
		for range 6 - len(ftree.perm) {
			ftree.perm = "0" + ftree.perm
		}
		cursor += zeroIndex + 20 + 1
		trees = append(trees, ftree)
		if cursor == len(out) {
			return trees
		}
	}
}

func resolveOfsDelta(baseObject []byte, refObject []byte) []byte {
	out := refObject
	_, newCursor, _ := calculateLengthFromVariableBytes(&out, 0)
	_, newCursor, _ = calculateLengthFromVariableBytes(&out, newCursor)
	newContent := []byte{}
	instructionProccessed := 0
	for {
		b := out[newCursor]
		newCursor++
		msb := isMSB(b)
		bits := getOctetFromByte(b)
		if msb {
			// Copy
			bytesConsumed := 0
			instructionProccessed++
			// +----------+---------+---------+---------+---------+-------+-------+-------+
			// | 1xxxxxxx | offset1 | offset2 | offset3 | offset4 | size1 | size2 | size3 |
			// +----------+---------+---------+---------+---------+-------+-------+-------+
			baseObjStartOffsetBits := ""
			CopySizeBits := ""
			offsetBits := bits[4:]
			lenghBits := bits[1:4]
			for ind := range 3 {
				offsetIndex := 3 - ind
				bit := offsetBits[offsetIndex]
				if string(bit) == "1" {
					nextBits := getOctetFromByte(out[newCursor+uint(bytesConsumed)])
					baseObjStartOffsetBits = nextBits + baseObjStartOffsetBits
					bytesConsumed++
				} else {
					baseObjStartOffsetBits = "00000000" + baseObjStartOffsetBits
				}
			}
			for ind := range 2 {
				lengthIndex := 2 - ind
				bit := lenghBits[lengthIndex]
				if string(bit) == "1" {
					nextBits := getOctetFromByte(out[newCursor+uint(bytesConsumed)])
					CopySizeBits = nextBits + CopySizeBits
					bytesConsumed++
				} else {
					CopySizeBits = "00000000" + CopySizeBits
				}
			}
			offset, err := strconv.ParseUint(baseObjStartOffsetBits, 2, 32)
			exitIfError(err, "PARSE_INT_O")
			copySize, err := strconv.ParseUint(CopySizeBits, 2, 32)
			exitIfError(err, "PARSE_INT_CSB")
			if copySize == 0 {
				// size zero is automatically converted to 0x10000 which is 65536 in Decimal
				copySize = 65536
			}
			start := int(offset)
			end := start + int(copySize)
			newContent = append(newContent, []byte(baseObject[start:end])...)
			newCursor += uint(bytesConsumed)
		} else {
			// Insert
			instructionProccessed++
			SizeToInsert, err := strconv.ParseUint(bits, 2, 32)
			exitIfError(err, "PARSEINT_SI")
			start := newCursor
			end := newCursor + uint(SizeToInsert)
			newContent = append(newContent, out[start:end]...)
			newCursor += uint(SizeToInsert)
		}
		if int(newCursor) == len(out) {
			return newContent
		}
	}
}
