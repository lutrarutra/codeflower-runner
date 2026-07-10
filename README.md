# codeflower-runner

CLI workflow runner for Code Flower App.

## Build

```bash
make build
```

The binary is output to `build/codeflower`.

## Usage

```bash
codeflower [command]
```

### Global flags

All commands (except `init`) require prior authentication via an API token.

---

## Commands

### `init`

Authenticate with the backend and save your API token locally.

```bash
codeflower init
# Prompts: Enter your API token: <paste-token>
```

Must be run before any other command.

---

### `auth`

Verify that your stored API token is still valid.

```bash
codeflower auth
# Successfully authenticated as <username>
```

---

### `pull`

Pull a tool's Docker image by ref, tool ID, or release ID.

```bash
# By ref string
codeflower pull --ref fastq2ubam:latest

# By tool UUID and release UUID
codeflower pull --tool-id <uuid> --release-id <uuid>

# Short flags
codeflower pull -r fastq2ubam:v1 -t <uuid> -s <uuid>
```

---

### `run`

Pull (if needed) and execute a tool release. Supports dynamic help that
shows available commands and parameters for the given ref.

```bash
# Show available commands/variants for a tool
codeflower run fastq2ubam

# Run the default variant of the default command
codeflower run fastq2ubam -- --input /path/to/file.fastq

# Specify a command and variant
codeflower run fastq2ubam align -- --r1 /data/R1.fastq.gz --r2 /data/R2.fastq.gz

# Force execution (skip validation)
codeflower run fastq2ubam -f -- --input file.fastq

# Skip image pull (use local image only)
codeflower run fastq2ubam --no-pull -- --input file.fastq
```

---

### `exec`

Resolve a tool ref to its full image digest and run it with `docker run`.
All flags and arguments after the ref are passed through to Docker.

```bash
# Run a tool with arguments
codeflower exec fastq2usam -- --r1 /path/to/R1.fastq.gz

# Mount a volume and pass arguments
codeflower exec fastq2usam -v /data:/data -- --r1 /data/R1.fastq.gz

# Set environment variables
codeflower exec fastq2usam -e THREADS=4 -- --r1 ./R1.fastq.gz
```

---

### `attach`

Like `exec`, but runs the container interactively (`docker run -it`).
Useful for debugging or exploring a container's environment.

```bash
# Run the tool's entrypoint interactively
codeflower attach fastq2usam

# Start a shell inside the container
codeflower attach fastq2usam -- sh

# Mount data and open a shell
codeflower attach fastq2usam -v /data:/data -- sh
```

---

### `start`

Fetch a workflow run from the backend, interactively prompt for required
file inputs, then execute every tool container locally in sequence.

```bash
codeflower start <run-uuid>

# Example
codeflower start 3b3c3b3c-3b3c-3b3c-3b3c-3b3c3b3c3b3c
```

The command will:
1. Fetch the run, workflow graph, and task parameter overrides from the API.
2. For each tool node, prompt for any unconnected input file paths.
3. Pull the required images and execute each container with resolved arguments.

---