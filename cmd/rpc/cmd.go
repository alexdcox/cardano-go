package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

const maxFileDescriptors = 10 // Adjust this based on your system's limits

type fdPool struct {
	fds    chan int
	mutex  sync.Mutex
	inUse  map[int]bool
	nextFd int
	maxFds int
}

func newFdPool(max int) *fdPool {
	return &fdPool{
		fds:    make(chan int, max),
		inUse:  make(map[int]bool),
		nextFd: 3, // Start from 3 as 0, 1, 2 are typically stdin, stdout, stderr
		maxFds: max,
	}
}

func (p *fdPool) get() (int, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if len(p.inUse) >= p.maxFds {
		return 0, fmt.Errorf("max file descriptors reached")
	}

	var fd int
	select {
	case fd = <-p.fds:
	default:
		fd = p.nextFd
		p.nextFd++
	}

	p.inUse[fd] = true
	return fd, nil
}

func (p *fdPool) release(fd int) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.inUse[fd] {
		delete(p.inUse, fd)
		select {
		case p.fds <- fd:
		default:
			// Channel is full, just discard
		}
	}
}

type InputData struct {
	Method string            `json:"method"`
	Params string            `json:"params"`
	Files  map[string]string `json:"files"`
}

type VirtualFile struct {
	Name    string
	Content []byte
}

func runCommandWithVirtualFiles(command string, params string, env []string, virtualFiles []VirtualFile) ([]byte, error) {
	pool := newFdPool(maxFileDescriptors)
	fileMap := make(map[string]int)
	cmd := exec.Command(command)
	cmd.Env = env

	for _, vf := range virtualFiles {
		r, w, err := os.Pipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create pipe for %s: %v", vf.Name, err)
		}
		defer r.Close()

		fd, err := pool.get()
		if err != nil {
			return nil, fmt.Errorf("failed to get file descriptor: %v", err)
		}
		defer pool.release(fd)

		cmd.ExtraFiles = append(cmd.ExtraFiles, r)
		fileMap[vf.Name] = fd

		go func(w *os.File, content []byte) {
			defer w.Close()
			io.Copy(w, bytes.NewReader(content))
		}(w, vf.Content)
	}

	// Replace file:// references with /dev/fd/X
	re := regexp.MustCompile(`file://(.*?)(?:\s|$)`)
	params = re.ReplaceAllStringFunc(params, func(match string) string {
		filename := strings.TrimPrefix(match, "file://")
		filename = strings.TrimSpace(filename)
		if fd, ok := fileMap[filename]; ok {
			return fmt.Sprintf("/dev/fd/%d ", fd)
		}
		return match // If no replacement found, return original match
	})

	// Split params into args
	cmd.Args = append(cmd.Args, strings.Fields(params)...)

	var out syncBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	fmt.Println("--------------------------------------------------")
	fmt.Println("CMD IN")
	fmt.Println(cmd.String())
	fmt.Println("--------------------------------------------------")

	err := cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start command: %v", err)
	}

	err = cmd.Wait()
	fmt.Println("--------------------------------------------------")
	fmt.Println("CMD OUT")
	fmt.Println(string(out.Bytes()))
	fmt.Println("--------------------------------------------------")
	if err != nil {
		return out.Bytes(), fmt.Errorf("command failed: %v", err)
	}

	return out.Bytes(), nil
}

type syncBuffer struct {
	buffer bytes.Buffer
	mutex  sync.Mutex
}

func (b *syncBuffer) Write(p []byte) (n int, err error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.Write(p)
}

func (b *syncBuffer) Bytes() []byte {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.buffer.Bytes()
}
