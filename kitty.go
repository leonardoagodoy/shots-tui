package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"strings"

	// decoders registrados para image.Decode
	_ "image/gif"
	_ "image/jpeg"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"golang.org/x/sys/unix"
)

// rect é uma área da tela em células, 1-indexada (como o CUP do terminal).
type rect struct{ col, row, cols, rows int }

var tty = func() *os.File {
	if f, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
		return f
	}
	return os.Stdout
}()

// cellPx devolve o tamanho de uma célula em pixels (fallback 10x20).
func cellPx() (float64, float64) {
	ws, err := unix.IoctlGetWinsize(int(tty.Fd()), unix.TIOCGWINSZ)
	if err == nil && ws.Xpixel > 0 && ws.Ypixel > 0 && ws.Col > 0 && ws.Row > 0 {
		return float64(ws.Xpixel) / float64(ws.Col), float64(ws.Ypixel) / float64(ws.Row)
	}
	return 10, 20
}

// imageDims lê só o cabeçalho da imagem.
func imageDims(path string) (w, h int, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}

// buildImage devolve a sequência de escape do protocolo gráfico do kitty que
// apaga a imagem anterior e desenha a nova, ajustada e centralizada em a.
func buildImage(path string, a rect) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return "", err
	}
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()

	cw, ch := cellPx()
	scale := math.Min(float64(a.cols)*cw/float64(iw), float64(a.rows)*ch/float64(ih))
	dw, dh := float64(iw)*scale, float64(ih)*scale
	dc := min(max(int(math.Round(dw/cw)), 1), a.cols)
	dr := min(max(int(math.Round(dh/ch)), 1), a.rows)
	col := a.col + (a.cols-dc)/2
	row := a.row + (a.rows-dr)/2

	// reduz antes de enviar: o terminal só precisa dos pixels que vai mostrar
	if tw, th := max(int(dw), 1), max(int(dh), 1); tw < iw {
		dst := image.NewRGBA(image.Rect(0, 0, tw, th))
		xdraw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
		img = dst
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	data := base64.StdEncoding.EncodeToString(buf.Bytes())

	var b strings.Builder
	b.WriteString("\x1b7") // salva o cursor
	b.WriteString(deleteImage)
	fmt.Fprintf(&b, "\x1b[%d;%dH", row, col)
	const chunk = 4096
	for i := 0; i < len(data); i += chunk {
		end := min(i+chunk, len(data))
		more := 1
		if end == len(data) {
			more = 0
		}
		if i == 0 {
			fmt.Fprintf(&b, "\x1b_Ga=T,f=100,i=1,c=%d,r=%d,C=1,q=2,m=%d;", dc, dr, more)
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d,q=2;", more)
		}
		b.WriteString(data[i:end])
		b.WriteString("\x1b\\")
	}
	b.WriteString("\x1b8") // restaura o cursor
	return b.String(), nil
}

const deleteImage = "\x1b_Ga=d,d=I,i=1,q=2\x1b\\"

func clearImage() { tty.WriteString(deleteImage) }
