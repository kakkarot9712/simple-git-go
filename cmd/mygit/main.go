package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Usage: your_program.sh <command> <arg1> <arg2> ...
func main() {
	permMap := map[string]uint32{"file": 100644, "exe": 100755, "symlink": 120000, "dir": 40000}
	BYTE_VAL := 128
	CWD, err := os.Getwd()
	flag.Parse()
	exitIfError(err, "fatal: cannot get current working directory")
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: mygit <command> [<args>...]\n")
		os.Exit(1)
	}

	switch command := flag.Arg(0); command {
	case "init":
		for _, dir := range []string{".git", ".git/objects", ".git/refs"} {
			err := os.MkdirAll(filepath.Join(CWD, dir), 0755)
			exitIfError(err, fmt.Sprintf("Error creating directory: %s\n", err))
		}

		headFileContents := []byte("ref: refs/heads/main\n")
		err := os.WriteFile(filepath.Join(CWD, ".git", "HEAD"), headFileContents, 0644)
		exitIfError(err, fmt.Sprintf("Error writing file: %s\n", err))
		fmt.Println("Initialized git directory")

	case "cat-file":
		if len(flag.Args()) < 3 {
			fmt.Fprintf(os.Stderr, "fatal: mygit cat-file: insufficient arguments passed\n")
			os.Exit(1)
		}
		allowedTypes := []string{"blob", "tree", "commit"}
		objectType := flag.Arg(1)
		sha := flag.Arg(2)
		objectPath := filepath.Join(CWD, ".git", "objects", sha[:2], sha[2:])
		buff, err := os.ReadFile(objectPath)
		exitIfError(err, fmt.Sprintf("Error reading file: %s\n", err))
		data, _ := decompressContent(buff)
		if objectType == "-p" {
			// Pretty print
			spaceIndex := bytes.Index(data, []byte(" "))
			objectType = string(data[:spaceIndex])
		} else {
			fmt.Fprintf(os.Stderr, "fatal: mygit cat-file: only -p mode is supported as of now\n")
			os.Exit(1)
		}
		if !slices.Contains(allowedTypes, objectType) {
			fmt.Fprintf(os.Stderr, "fatal: unsupported object type detected\n")
			os.Exit(1)
		}
		// if !bytes.HasPrefix(data, []byte(objectType+" ")) {
		// 	fmt.Fprintf(os.Stderr, "fatal: mygit cat-file %s: bad file\n", sha)
		// 	os.Exit(1)
		// }
		switch objectType {
		case "blob":
			data = decodeBlobObject(data, false)
			os.Stdout.Write(data)
		case "tree":
			trees := decodeTreeObject(data, false)
			for _, tree := range trees {
				var oType string
				switch tree.perm {
				case DIR:
					oType = "tree"
				default:
					oType = "blob"
				}
				os.Stdout.Write([]byte(fmt.Sprintf("%s %s %s\t%s\n", tree.perm, oType, hex.EncodeToString(tree.sha[:]), tree.name)))
			}
		default:
			zeroIndex := bytes.Index(data, []byte{0x0})
			os.Stdout.Write(data[zeroIndex+1:])
		}

	case "hash-object":
		if len(flag.Args()) < 2 {
			fmt.Fprintf(os.Stderr, "fatal: mygit hash-object: insufficient arguments passed\n")
			os.Exit(1)
		}
		fileName := flag.Arg(1)
		if flag.Arg(1) == "-w" {
			// Actually Write to Object store
			fileName = flag.Arg(2)
		}
		if fileName == "" {
			fmt.Fprintf(os.Stderr, "fatal: mygit hash-object: invalid filename passed\n")
			os.Exit(1)
		}
		buff, err := os.ReadFile(filepath.Join(CWD, fileName))
		exitIfError(err, fmt.Sprintf("Error reading file: %s", err))
		blob := writeHeaderToContent(buff, Blob)
		hexHash := hex.EncodeToString(hashContent(blob))
		if flag.Arg(1) == "-w" {
			createBlobObject(buff)
		}
		os.Stdout.Write([]byte(hexHash))

	case "ls-tree":
		supportedSubArgs := []string{"--name-only", "--object-only"}
		if len(flag.Args()) < 2 {
			fmt.Fprintf(os.Stderr, "fatal: mygit ls-tree: insufficient arguments passed\n")
			os.Exit(1)
		}
		treeSha := flag.Arg(1)
		if slices.Contains(supportedSubArgs, treeSha) {
			// Only print name
			treeSha = flag.Arg(2)
			if slices.Contains(supportedSubArgs, treeSha) {
				fmt.Fprintf(os.Stderr, "fatal: mygit ls-tree: no duplicate sub args or multiple sub args can't be combined!\n")
				os.Exit(1)
			}
			if treeSha == "" {
				fmt.Fprintf(os.Stderr, "fatal: mygit ls-tree: no object hash passed\n")
				os.Exit(1)
			}
		}
		treePath := ".git/objects/" + treeSha[:2] + "/" + treeSha[2:]
		data, err := os.ReadFile(treePath)
		exitIfError(err, fmt.Sprintf("Error reading file: %s", err))
		trees := decodeTreeObject(data, true)
		if flag.Arg(1) == "--name-only" {
			for _, t := range trees {
				fmt.Println(t.name)
			}
		} else if flag.Arg(1) == "--object-only" {
			for _, t := range trees {
				fmt.Println(hex.EncodeToString(t.sha[:]))
			}
		} else {
			for _, t := range trees {
				var oType string
				switch t.perm {
				case DIR:
					oType = "tree"
				default:
					oType = "blob"
				}
				os.Stdout.Write([]byte(fmt.Sprintf("%s %s %s\t%s\n", t.perm, oType, hex.EncodeToString(t.sha[:]), t.name)))
			}
		}

	case "write-tree":
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
		* PackData:
		* 	94 ||| 0F -> 1 | 001 | 0100  |||  0 | 000 | 1111
		*
		*   bin(start) -> MSB | Object Type  | Number ()
		*		Legnth of Object: 263 bytes after inflation (Decompression)
		*		Type of Object: 3 (Blob)
		*
		*	Other bytes -> MSB |
		*		MSB: Wether nect byte is part of current integer
		* 		Check: if byte is less than 128, which is 10000000
		*
		*   Object Type: See below list
		*
		* - OBJ_COMMIT (1)
		* - OBJ_TREE (2)
		* - OBJ_BLOB (3)
		* - OBJ_TAG (4) ---> Not supported as of now
		* - OBJ_OFS_DELTA (6)
		* - OBJ_REF_DELTA (7) ---> Not supported as of now
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
			CurrentNegativeOffsetToBO := 0
			latestCommitHex := ""
			for {
				b := packData[cursor]
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
						} else {
							b = packData[cursor]
							cursor++
							CurrentObjectLengthBits = getLengthBitsFromByte(b, false) + CurrentObjectLengthBits
						}
						VariableLengthBytesProcessed++
						isMSB := isMSB(b)
						if !isMSB {
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

					ofsObject := ofsRefObject{baseObjectIndex: CurrentOffsetObjectStart - CurrentNegativeOffsetToBO, currentObjectIndex: CurrentObjectStartIndex}
					out, unreadBuffLen := decompressContent([]byte(packData[cursor:]))
					ofsObject.object = out
					ofsRefDeltas = append(ofsRefDeltas, ofsObject)
					cursor += len(packData[cursor:]) - unreadBuffLen
					CurrentProccessingStatus = HeaderProcessingStart
					CurrentObjectStartIndex = 0
					CurrentObjectType = Unsepcified
					CurrentOffsetObjectStart = -1
					CurrentNegativeOffsetToBO = 0
				}
				if cursor == len(packData) {
					fmt.Println("Resolving Deltas...")
					for _, delta := range ofsRefDeltas {
						baseObjectRef := objectRefs[delta.baseObjectIndex]
						if baseObjectRef.ObjectType == OFSDelta {
							baseObjectRef = objectRefs[baseObjectRef.BaseObjectIndex]
							if baseObjectRef.ObjectType == OFSDelta {
								log.Fatal("RECURSE_DELTA_MULTEDEP: ", baseObjectRef.BaseObjectIndex)
							}
						}
						content := resolveOfsDelta(objects[baseObjectRef.Hash].content, delta.object)
						hexHash := hex.EncodeToString(hashContent(writeHeaderToContent(content, baseObjectRef.ObjectType)))
						objects[hexHash] = gitObject{
							objectType: baseObjectRef.ObjectType,
							content:    content,
						}
						objectRefs[delta.currentObjectIndex] = objectRef{
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
						os.MkdirAll(filepath.Join(CWD, dest, ".git", "objects"), 0644)
						os.MkdirAll(filepath.Join(CWD, dest, ".git", "refs", "heads"), 0644)
						os.WriteFile(filepath.Join(CWD, dest, ".git", "HEAD"), []byte("ref:"+symRef), 0755)
						os.WriteFile(filepath.Join(CWD, dest, ".git", "refs", "heads", branchName), []byte(latestCommitHex), 0755)
						data := fmt.Sprintf(`[remote "origin"]
	url = %v
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "%v"]
	remote = origin
	merge = refs/heads/%v
	vscode-merge-base = origin/%v
`, gitUrl, branchName, branchName, branchName)
						os.WriteFile(filepath.Join(CWD, dest, ".git", "config"), []byte(data), 0755)
						treeContent := writeHeaderToContent(latestTree, Tree)
						trees := decodeTreeObject(treeContent, false)
						var writeTree func(string, []tree)
						for hexHash, obj := range objects {
							writeObjectToDisk(writeHeaderToContent(obj.content, obj.objectType), hexHash, true, filepath.Join(CWD, dest))
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
									err = os.WriteFile(filepath.Join(".", rootPath, tree.name), objects[hexHash].content, 0755)
									if err != nil {
										panic(err)
									}
								} else if tree.perm == "040000" {
									treeObject := objects[hexHash].content
									treeContent := writeHeaderToContent(treeObject, Tree)
									latestTrees := decodeTreeObject(treeContent, false)
									writeTree(filepath.Join(".", rootPath, tree.name), latestTrees)
								} else {
									fmt.Println("Unhandled tree:", tree)
								}
							}
						}
						writeTree(dest, trees)
						fmt.Println("Done!")
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
