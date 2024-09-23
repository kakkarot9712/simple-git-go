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
			fmt.Println("Checksum verified! Resolving Objects...")

			// Stores mapping of index to Hashed Objects
			objectRefs := map[int]objectRef{}

			// Stores mapping of hash to respective raw objects
			objects := map[string]gitObject{}
			ofsRefDeltas := []ofsRefObject{}

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
							if CurrentObjectType == OFSDelta {
								CurrentProccessingStatus = DeltifiedObjBasePtrExtractionStarts
								CurrentOffsetObjectStart = cursor - VariableLengthBytesProcessed
							} else if CurrentObjectType == REFDelta {
								log.Fatal("Repository with RefData is not supported as of now!")
							} else {
								CurrentProccessingStatus = UndeltifiedObjectExtractionStarts
							}
							break
						}
					}
				} else if CurrentProccessingStatus == UndeltifiedObjectExtractionStarts {
					out, unreadBuffLen := decompressContent([]byte(packData[cursor:]))
					hexHash := hex.EncodeToString(hashContent(writeHeaderToContent(out, CurrentObjectType)))
					objects[hexHash] = gitObject{
						objectType: CurrentObjectType,
						content:    out,
					}
					objectRefs[CurrentObjectStartIndex] = objectRef{
						Hash:       hexHash,
						ObjectType: CurrentObjectType,
					}
					if CurrentObjectType == Commit && latestCommitHex == "" {
						latestCommitHex = hexHash
					}
					CurrentProccessingStatus = HeaderProcessingStart
					cursor += len(packData[cursor:]) - unreadBuffLen
					CurrentObjectStartIndex = 0
					CurrentObjectType = Unsepcified

				} else if CurrentProccessingStatus == DeltifiedObjBasePtrExtractionStarts {
					VariableLengthBytesProcessed := 0
					CurrentNegativeOffsetToBOBits := ""
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
							objectRefs[CurrentOffsetObjectStart] = objectRef{
								ObjectType:      CurrentObjectType,
								BaseObjectIndex: CurrentOffsetObjectStart - CurrentNegativeOffsetToBO,
							}
							break
						} else {
							VariableLengthBytesProcessed++
						}
					}

					ofsObject := ofsRefObject{baseObjectIndex: CurrentOffsetObjectStart - CurrentNegativeOffsetToBO}
					// fmt.Println("OFS with start of", CurrentOffsetObjectStart, "at index", CurrentOffsetObjectStart-CurrentNegativeOffsetToBO, cursor)
					out, unreadBuffLen := decompressContent([]byte(packData[cursor:]))
					ofsObject.object = out
					ofsRefDeltas = append(ofsRefDeltas, ofsObject)
					cursor += len(packData[cursor:]) - unreadBuffLen
					CurrentProccessingStatus = HeaderProcessingStart
					// CurrentObjectLength = -1
					CurrentObjectStartIndex = 0
					CurrentObjectType = Unsepcified
					CurrentOffsetObjectStart = -1
					// CurrentObjectLength = -1
					CurrentNegativeOffsetToBO = 0
					// fmt.Println(sourceLength, targetLength, newCursor, "STLN")
					// fmt.Println("OFS_PROC", CurrentNegativeOffsetToBO, CurrentOffsetObjectStart)
				}
				if cursor == len(packData) {
					fmt.Println("Resolving Deltas...")
					for _, delta := range ofsRefDeltas {
						baseObjectRef := objectRefs[delta.baseObjectIndex]
						// fmt.Println(delta.baseObjectIndex, "BOI")
						if baseObjectRef.ObjectType == OFSDelta {
							baseObjectRef = objectRefs[baseObjectRef.BaseObjectIndex]
						}
						content := resolveOfsDelta(objects[baseObjectRef.Hash].content, delta.object)
						hexHash := hex.EncodeToString(hashContent(writeHeaderToContent(content, baseObjectRef.ObjectType)))
						objects[hexHash] = gitObject{
							objectType: baseObjectRef.ObjectType,
							content:    content,
						}
						objectRefs[delta.baseObjectIndex] = objectRef{
							Hash:            hexHash,
							ObjectType:      baseObjectRef.ObjectType,
							BaseObjectIndex: 0,
						}
					}
					proccessedObjectLength := len(objects)
					if proccessedObjectLength == int(objectsLength) {
						latestCommit := string(objects[latestCommitHex].content)
						latestTree := objects[latestCommit[5:45]].content
						splits := strings.Split(symRef, "/")
						branchName := splits[len(splits)-1]
						os.MkdirAll(path.Join(CWD, dest, ".git", "objects"), 0644)
						os.MkdirAll(path.Join(CWD, dest, ".git", "refs", "heads"), 0644)
						os.WriteFile(path.Join(CWD, dest, ".git", "HEAD"), []byte("ref:"+symRef), 0755)
						os.WriteFile(path.Join(CWD, dest, ".git", "refs", "heads", branchName), []byte(latestCommitHex), 0755)
						treeContent := writeHeaderToContent(latestTree, Tree)
						trees := decodeTreeObject(treeContent, false)
						var writeTree func(string, []tree)
						for hexHash, obj := range objects {
							writeObjectToDisk(writeHeaderToContent(obj.content, obj.objectType), hexHash, true, path.Join(CWD, dest))
						}
						writeTree = func(destination string, trees []tree) {
							for _, tree := range trees {
								hexHash := hex.EncodeToString(tree.sha[:])
								rootPath := destination
								if tree.perm == "100644" {
									err := os.MkdirAll(rootPath, 0644)
									if err != nil {
										panic(err)
									}
									err = os.WriteFile(path.Join(".", rootPath, tree.name), objects[hexHash].content, 0755)
									if err != nil {
										panic(err)
									}
								} else if tree.perm == "40000" {
									treeObject := objects[hexHash].content
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
						log.Fatal("Length mismatch detected!", proccessedObjectLength, objectsLength)
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
