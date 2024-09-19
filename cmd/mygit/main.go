package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"path"
	"strconv"
	"strings"

	// Uncomment this block to pass the first stage!

	"os"
	// ""
)

// Usage: your_program.sh <command> <arg1> <arg2> ...
func main() {
	permMap := map[string]uint32{"file": 100644, "exe": 100755, "symlink": 120000, "dir": 40000}
	BYTE_VAL := 128
	CWD, err := os.Getwd()
	exitIfError(err, "CWD")
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	// fmt.Println("Logs from your program will appear here!")

	// Uncomment this block to pass the first stage!
	//
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
		exitIfError(err, "FILE_READ_CAT")
		data := decodeBlobObject(buff)
		os.Stdout.Write(data)

	case "hash-object":
		fileName := os.Args[3]
		buff, err := os.ReadFile("./" + fileName)
		exitIfError(err, "File Read")
		hexHash := createBlobObject(buff)
		os.Stdout.Write([]byte(hexHash))

	case "ls-tree":
		treeSha := os.Args[3]
		// Create Path to tree in .git using treeSHA and Read the file
		treePath := ".git/objects/" + treeSha[:2] + "/" + treeSha[2:]
		data, err := os.ReadFile(treePath)
		exitIfError(err, "READ_FILE")
		trees := decodeTreeObject(data, true)
		for _, t := range trees {
			fmt.Println(t.name)
		}

	case "write-tree":
		// data, err := os.ReadFile(path.Join(CWD, ".gitignore"))
		// if err != nil {
		// 	fmt.Println("read .gitignore failed! skipping ignorelist")
		// }
		// ignoreList := strings.Split(string(data), "\n")
		// ignoreList = append(ignoreList, GIT_HOME)
		hash := createTreeObject(".", &permMap)
		hexHash := hex.EncodeToString(hash)
		os.Stdout.Write([]byte(hexHash))

	case "commit-tree":
		hash := os.Args[2]
		parent := os.Args[4]
		message := os.Args[6]
		author_time := 1724929752
		tz := "+0530"
		authorName := "Vikalp Gandha"
		email := "vikalp.gandha@test.com"

		commitContent := "tree " + hash + "\n" +
			"parent " + parent + "\n" +
			"author " + authorName + " " + "<" + email + ">" + " " + strconv.Itoa(author_time) + " " + tz + "\n" +
			"committer " + authorName + " " + "<" + email + ">" + " " + strconv.Itoa(author_time) + " " + tz + "\n" +
			"\n" +
			message + "\n"
		commitHex := createCommitObject([]byte(commitContent))
		os.Stdout.Write([]byte(commitHex))

	case "clone":
		gitUrl := os.Args[2]
		dest := os.Args[3]
		// Peform Ref Discovery
		fmt.Println("Cloning repository in", dest)
		refs := discoverRefs(gitUrl)

		// Get Default Branch SHA
		defaultBranchSha, symRef := getDefaultBranchFromRefs(refs)

		// Get Pack File of default branch
		packData := getPackDataFromBranchSha(gitUrl, defaultBranchSha)

		/*
			*  ###################### PACK FILE ###########################
			*
			*                  P  A  C  K |   Version   | Objects Nos
			* Start Bytes --> 50 41 43 4B | 00 00 00 02 | 00 00 01 4C | Start of the objects...
			* 332 Objects
			*
			* - OBJ_COMMIT (1)
			* - OBJ_TREE (2)
			* - OBJ_BLOB (3)
			* - OBJ_TAG (4)
			* - OBJ_OFS_DELTA (6)
			* - OBJ_REF_DELTA (7)
			*
			* PackData:
			* 	94 ||| 0F -> 1 | 001 | 0100  |||  0 | 000 | 1111
			*
			*   bin(start) -> MSB | Object Type  | Number ()
					Legnth of Object: 263 bytes after inflation (Decompression)
					Type of Object: 3 (Blob)
			*
				Other bytes -> MSB |
				MSB: Wether nect byte is part of current integer
			* 		Check: if byte is less than 128, which is 10000000
			*
			*   Object Type: See above for list
			*
		*/
		version, objectsLength := getPackFileMetadata(packData)
		if version == 2 {
			// Verify checksum
			fmt.Println("Verifying Packfile Cheksum...")
			expectedSHA1Sum := []byte(packData[len(packData)-20:])
			packData = packData[:len(packData)-20]
			shaSum := hashContent([]byte(packData))
			if string(expectedSHA1Sum) != string(shaSum) {
				fmt.Println("Packfile checksum validation failed! Aborting...")
				os.Exit(1)
			}
			fmt.Println("Checksum verified! Resolving Deltas...")

			// Stores mapping of index to Hashed Objects
			objectRefs := map[int]objectRef{}

			// Stores mapping of hash to respective raw objects
			blobObjects := map[string][]byte{}
			treeObjects := map[string][]byte{}
			commitObjects := map[string][]byte{}

			cursor := 12
			CurrentObjectStartIndex := 0
			CurrentProccessingStatus := HeaderProcessingStart
			CurrentOffsetObjectStart := -1

			CurrentObjectType := Unsepcified
			// CurrentObjectLength := -1

			CurrentNegativeOffsetToBO := 0
			latestCommitHex := ""

			for {
				b := packData[cursor]
				// Start
				if CurrentProccessingStatus == HeaderProcessingStart {
					CurrentObjectLengthBits := ""
					VariableLengthBytesProcessed := 0
					for {
						if CurrentProccessingStatus == HeaderProcessingStart {
							CurrentObjectStartIndex = cursor
							cursor++
							CurrentObjectType = getObjectTypeFromMSB(b)
							CurrentObjectLengthBits = getLengthBitsFromByte(b, true)
							CurrentProccessingStatus = HeaderProcessing
							// fmt.Println("HPS at", CurrentObjectStartIndex, "type", CurrentObjectType, CurrentObjectLengthBits)
						} else {
							b = packData[cursor]
							cursor++
							CurrentObjectLengthBits = getLengthBitsFromByte(b, false) + CurrentObjectLengthBits
						}
						VariableLengthBytesProcessed++
						isMSB := isMSB(b)
						if !isMSB {
							// Last Header Byte

							// fmt.Println("Last H Byte:", b, cursor, CurrentObjectLengthBits, CurrentObjectType)
							// objectLength, err := strconv.ParseUint(CurrentObjectLengthBits, 2, 32)
							// exitIfError(err, "LEN_PARSEUINT")
							// CurrentObjectLength = int(objectLength)
							if CurrentObjectType == OFSDelta || CurrentObjectType == REFDelta {
								CurrentProccessingStatus = DeltifiedObjBasePtrExtractionStarts
								CurrentOffsetObjectStart = cursor - VariableLengthBytesProcessed
							} else {
								CurrentProccessingStatus = UndeltifiedObjectExtractionStarts
							}
							break
						}
					}
				} else if CurrentProccessingStatus == UndeltifiedObjectExtractionStarts {
					out, unreadBuffLen := decompressContent([]byte(packData[cursor:]))
					hexHash := ""
					if CurrentObjectType == Commit {
						hexHash = hex.EncodeToString(hashContent(writeHeaderToContent(out, Commit)))
						// hexHash = createCommitObject(string(out))
						commitObjects[hexHash] = out
						if latestCommitHex == "" {
							latestCommitHex = hexHash
						}
						// fmt.Println("Wrote Object ", hexHash)
					} else if CurrentObjectType == Blob {
						// Raw Blob Obejct
						hexHash = hex.EncodeToString(hashContent(writeHeaderToContent(out, Blob)))
						// hexHash = createBlobObject(out)
						blobObjects[hexHash] = out
						// fmt.Println("Wrote Object ", hexHash)
					} else if CurrentObjectType == Tree {
						// treeContent := "tree " + strconv.Itoa(len(out)) + string(byte(0)) + string(out)
						// hash := hashContent([]byte(treeContent))
						// hexHash = hex.EncodeToString(hash)
						hexHash = hex.EncodeToString(hashContent(writeHeaderToContent(out, Tree)))
						// writeObjectToDisk([]byte(treeContent), hexHash, true)
						treeObjects[hexHash] = out
						// fmt.Println("Wrote Object ", hexHash)
					}
					objectRefs[CurrentObjectStartIndex] = objectRef{
						Hash:       hexHash,
						ObjectType: CurrentObjectType,
					}
					CurrentProccessingStatus = HeaderProcessingStart
					cursor += len(packData[cursor:]) - unreadBuffLen
					// CurrentObjectLength = -1
					CurrentObjectStartIndex = 0
					CurrentObjectType = Unsepcified

				} else if CurrentProccessingStatus == DeltifiedObjBasePtrExtractionStarts {
					VariableLengthBytesProcessed := 0
					CurrentNegativeOffsetToBOBits := ""
					CurrentObjectStartIndex = cursor - 2
					for {
						b := packData[cursor]
						isMSB := isMSB(b)
						cursor++
						CurrentNegativeOffsetToBOBits += getLengthBitsFromByte(b, false)
						if VariableLengthBytesProcessed > 0 {
							CurrentNegativeOffsetToBO += int(math.Pow(float64(BYTE_VAL), float64(VariableLengthBytesProcessed)))
						}
						if !isMSB {
							converted, err := strconv.ParseUint(CurrentNegativeOffsetToBOBits, 2, 32)
							exitIfError(err, "OFS_END_CONV")
							CurrentNegativeOffsetToBO += int(converted)
							CurrentProccessingStatus = DeltaObjectExtract
							break
						} else {
							VariableLengthBytesProcessed++
						}
					}
				} else if CurrentProccessingStatus == DeltaObjectExtract {
					baseObjectRef := objectRefs[CurrentOffsetObjectStart-CurrentNegativeOffsetToBO]
					baseObject := []byte{}
					switch baseObjectRef.ObjectType {
					case Commit:
						baseObject = commitObjects[baseObjectRef.Hash]
					case Tree:
						baseObject = treeObjects[baseObjectRef.Hash]
					case Blob:
						baseObject = blobObjects[baseObjectRef.Hash]
					}
					// fmt.Println("base object", baseObjectRef)
					out, unreadBuffLen := decompressContent([]byte(packData[cursor:]))
					cursor += len(packData[cursor:]) - unreadBuffLen
					_, newCursor, _ := calculateLengthFromVariableBytes(&out, 0)
					_, newCursor, _ = calculateLengthFromVariableBytes(&out, newCursor)
					// fmt.Println(sourceLength, targetLength, newCursor, "STLN")
					// fmt.Println("OFS_PROC", CurrentNegativeOffsetToBO, CurrentOffsetObjectStart)
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
							// baseObjectRef := blobObjects[CurrentOffsetObjectStart-CurrentNegativeOffsetToBO]
							start := int(offset)
							end := start + int(copySize)
							// fmt.Println(start, ":", end, "C", b, len(baseObject))
							newContent = append(newContent, []byte(baseObject[start:end])...)
							newCursor += uint(bytesConsumed)
						} else {
							// Insert
							instructionProccessed++
							SizeToInsert, err := strconv.ParseUint(bits, 2, 32)
							exitIfError(err, "PARSEINT_SI")
							start := newCursor
							end := newCursor + uint(SizeToInsert)
							// fmt.Println(start, ":", end, "I", b)
							newContent = append(newContent, out[start:end]...)
							newCursor += uint(SizeToInsert)
							// fmt.Println("#########################", newCursor, len(out), "I")
						}
						if int(newCursor) == len(out) {
							// fmt.Println("#########################", newCursor, CurrentObjectStartIndex, len(out), cursor, "FIN")
							hexHash := ""
							switch baseObjectRef.ObjectType {
							case Commit:
								hexHash = hex.EncodeToString(hashContent(writeHeaderToContent(newContent, Commit)))
								// hexHash = createCommitObject(string(newContent))
								commitObjects[hexHash] = newContent
								// fmt.Println("Wrote Object ", hexHash)
							case Blob:
								hexHash = hex.EncodeToString(hashContent(writeHeaderToContent(newContent, Blob)))
								blobObjects[hexHash] = newContent

								// fmt.Println("Wrote Object ", hexHash)
							case Tree:
								// treeContent := "tree " + strconv.Itoa(len(newContent)) + string(byte(0)) + string(newContent)
								// hash := hashContent([]byte(treeContent))
								// hexHash = hex.EncodeToString(hash)
								// writeObjectToDisk([]byte(treeContent), hexHash, true)
								hexHash = hex.EncodeToString(hashContent(writeHeaderToContent(newContent, Tree)))
								treeObjects[hexHash] = newContent
								// fmt.Println("Wrote Object ", hexHash)
							}
							objectRefs[CurrentObjectStartIndex] = objectRef{
								Hash:       hexHash,
								ObjectType: baseObjectRef.ObjectType,
							}
							CurrentProccessingStatus = HeaderProcessingStart
							// CurrentObjectLength = -1
							CurrentObjectStartIndex = 0
							CurrentObjectType = Unsepcified
							CurrentOffsetObjectStart = -1
							// CurrentObjectLength = -1
							CurrentNegativeOffsetToBO = 0
							break
						}
					}
				}
				if cursor == len(packData) {
					proccessedObjectLength := len(blobObjects) + len(treeObjects) + len(commitObjects)
					if proccessedObjectLength == int(objectsLength) {
						latestCommit := string(commitObjects[latestCommitHex])
						latestTree := treeObjects[latestCommit[5:45]]
						fmt.Println("Writing Objects...")
						splits := strings.Split(symRef, "/")
						branchName := splits[len(splits)-1]
						os.MkdirAll(path.Join(CWD, dest, ".git", "objects"), 0644)
						os.MkdirAll(path.Join(CWD, dest, ".git", "refs", "heads"), 0644)
						os.WriteFile(path.Join(CWD, dest, ".git", "HEAD"), []byte("ref:"+symRef), 0755)
						os.WriteFile(path.Join(CWD, dest, ".git", "refs", "heads", branchName), []byte(latestCommitHex), 0755)
						treeContent := writeHeaderToContent(latestTree, Tree)
						trees := decodeTreeObject(treeContent, false)
						var writeTree func(string, []tree)
						for hexHash, blob := range blobObjects {
							writeObjectToDisk(writeHeaderToContent(blob, Blob), hexHash, true, path.Join(CWD, dest))
						}
						for hexHash, tree := range treeObjects {
							writeObjectToDisk(writeHeaderToContent(tree, Tree), hexHash, true, path.Join(CWD, dest))
						}
						for hexHash, commit := range commitObjects {
							writeObjectToDisk(writeHeaderToContent(commit, Commit), hexHash, true, path.Join(CWD, dest))
						}
						fmt.Println("Resolving Files...")
						writeTree = func(destination string, trees []tree) {
							for _, tree := range trees {
								hexHash := hex.EncodeToString(tree.sha[:])
								rootPath := destination
								if tree.perm == "100644" {
									err := os.MkdirAll(rootPath, 0644)
									if err != nil {
										panic(err)
									}
									err = os.WriteFile(path.Join(".", rootPath, tree.name), blobObjects[hexHash], 0755)
									if err != nil {
										panic(err)
									}
								} else if tree.perm == "40000" {
									treeObject := treeObjects[hexHash]
									treeContent := writeHeaderToContent(treeObject, Tree)
									latestTrees := decodeTreeObject(treeContent, false)
									writeTree(path.Join(".", rootPath, tree.name), latestTrees)
								} else {
									fmt.Println("Unhandled tree:", tree)
								}
							}
						}
						writeTree(dest, trees)
						fmt.Println("Done!")
						// fmt.Println("Length verified! looks all good!")
					} else {
						fmt.Println("Length mismatch detected!", proccessedObjectLength, objectsLength)
					}
					return
				}
			}
		} else {
			log.Fatal("This program only supports version 2 packfile as of now")
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %s\n", command)
		os.Exit(1)
	}
}
