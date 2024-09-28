# Simple Git

A lightweight implementation of Git, supporting basic Git operations. This project is written in Go and provides a command-line interface for interacting with Git-like repositories.

**Note**: This project is part of `Build Your Own X` challenge of CodeCrafters platform. Head over to
[codecrafters.io](https://codecrafters.io) to try the challenge.

## Supported Commands

- `ls-tree`: List the contents of a tree object
- `cat-file`: Display the contents of a Git object
- `commit-tree`: Create a new commit object from Tree Hash (Partially Supported)
- `write-tree`: Create a tree object from the current index.
- `clone`: Clone a repository (supports HTTP URLs only, limitations apply)
- `hash-object`: Compute object ID and optionally create a blob from a file.
- `init`: Create an empty Git repository or reinitialize an existing one.
- `config`: Create and/or update global config file. (Partially Supported)

## Prerequisites

This project requires Go to be installed on your system. If you don't have Go installed, you can download it from the official Go downloads page:

[https://go.dev/dl/](https://go.dev/dl/)

Choose the appropriate version for your operating system and follow the installation instructions provided on the Go website.

## Installation

After ensuring Go is installed on your system, follow these steps to set up Simple Git:

1. Clone the repository:
   ```
   git clone https://github.com/kakkarot9712/simple-git-go.git
   ```

2. Navigate to the project directory:
   ```
   cd simple-git-go
   ```

3. Build the project:
   ```
   go build ./cmd/mygit
   ```

This will create an executable named `mygit` in your project directory.

## Usage

After building the project, you can use the `mygit` executable to run Git commands. Here are some examples:

1. Initialize a new repository:
   ```
   ./mygit init
   ```

2. Create a blob object:
   ```
   ./mygit hash-object (-w) <file>
   ```

3. Display the contents of a Git object:
   ```
   ./mygit cat-file (-p) <object-hash>
   ```

4. Clone a repository:
   ```
   ./mygit clone <repository-url>
   ```

5. List the contents of a tree object:
   ```
   ./mygit ls-tree [--object-only | --name-only] <tree-hash>
   ```

6. Create a new commit:
   ```
   ./mygit commit-tree <tree-hash> -p <parent-commit-hash> -m "Commit message"
   ```

7. Manage global config file:
   ```
   ./mygit config [--global] [--add | --get] <key> <value>
   ```

## Limitations

- The `clone` command only supports HTTP URLs and v2 PackFiles.
- Cloning repositories with PackFiles containing Ref Delta objects are not supported. (Basically repositories with larger sizes are not supported).
- There is no `staging area` implimented in this CLI as of now.
- Currently `commit-tree` commands takes IST Timezone in commits (+0530) regardless of actual location.
- `commit-tree` only supports one line messages as of now.
- `config` command will only work with global config files named `.mygitconfig` to prevent unwanted changes to actual `.gitconfig` file. This config file will be fetched or created at `$USERPROFILE` directory if used in windows and in `$HOME` directory if used in Linux.
- Though you can set any value with section using `mygit config` command, This CLI will only use `user.name` and `user.email` from this config file as of now.