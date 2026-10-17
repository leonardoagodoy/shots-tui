package main

import (
	"fmt"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type item struct {
	path, name string
	mod        time.Time
	size       int64
}

var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true}

// scan lista as imagens de dir, das mais novas para as mais antigas.
func scan(dir string) ([]item, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var items []item
	for _, e := range entries {
		if e.IsDir() || !imageExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, item{filepath.Join(dir, e.Name()), e.Name(), info.ModTime(), info.Size()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
	return items, nil
}

func openFile(path string) error {
	cmd := exec.Command("xdg-open", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// copyImage põe o arquivo (como imagem) no clipboard.
func copyImage(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	var cmd *exec.Cmd
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		cmd = exec.Command("wl-copy", "-t", mt)
	} else {
		cmd = exec.Command("xclip", "-selection", "clipboard", "-t", mt)
	}
	cmd.Stdin = f
	return cmd.Run()
}

func trash(paths []string) error {
	out, err := exec.Command("gio", append([]string{"trash", "--"}, paths...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// renameFile troca o nome mantendo a extensão; devolve o novo caminho.
func renameFile(path, newBase string) (string, error) {
	newBase = strings.TrimSpace(newBase)
	if newBase == "" || strings.ContainsRune(newBase, '/') {
		return "", fmt.Errorf("nome inválido")
	}
	dst := filepath.Join(filepath.Dir(path), newBase+filepath.Ext(path))
	if dst == path {
		return path, nil
	}
	if _, err := os.Stat(dst); err == nil {
		return "", fmt.Errorf("já existe: %s", filepath.Base(dst))
	}
	return dst, os.Rename(path, dst)
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.0f%cB", float64(n)/float64(div), "KMGT"[exp])
}
