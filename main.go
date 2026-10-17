// shots — TUI para gerenciar screenshots (Bubble Tea + protocolo gráfico do kitty).
// Uso: shots [diretório]   (padrão: $SHOTS_DIR ou ~/Pictures/Screenshots)
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// paleta (style-guide: Vaulted)
var (
	cAccent  = lipgloss.Color("#ee5396")
	cText    = lipgloss.Color("#dde1e6")
	cStrong  = lipgloss.Color("#f2f4f8")
	cMuted   = lipgloss.Color("#8d8d8d")
	cBorder  = lipgloss.Color("#2e2e2e")
	cSurface = lipgloss.Color("#262626")
)

type mode int

const (
	modeList mode = iota
	modeRename
	modeTrash
	modeFull
)

type (
	showMsg struct{ seq int }
	imgMsg  struct {
		seq int
		esc string
	}
	statusMsg      struct{ text string }
	clearStatusMsg struct{ id int }
)

type model struct {
	dir   string
	all   []item
	shown []int // índices de all que passam no filtro
	cur   int
	off   int
	sel   map[string]bool

	w, h      int
	mode      mode
	preview   bool
	searching bool
	filter    textinput.Model
	input     textinput.Model // renomear
	err       string

	status   string
	statusID int
	dims     map[string]string

	seq     int    // geração do preview, descarta resultados antigos
	wantKey string // imagem que deveria estar na tela
}

func newInput(prompt string) textinput.Model {
	t := textinput.New()
	t.Prompt = prompt
	t.PromptStyle = lipgloss.NewStyle().Foreground(cAccent)
	t.TextStyle = lipgloss.NewStyle().Foreground(cText)
	t.Cursor.Style = lipgloss.NewStyle().Foreground(cAccent)
	return t
}

func newModel(dir string) *model {
	m := &model{dir: dir, sel: map[string]bool{}, preview: true, dims: map[string]string{},
		filter: newInput("/ "), input: newInput("")}
	m.reload("")
	return m
}

func (m *model) Init() tea.Cmd { return nil }

// ── dados ───────────────────────────────────────────────────────────────────

func (m *model) current() (item, bool) {
	if m.cur < 0 || m.cur >= len(m.shown) {
		return item{}, false
	}
	return m.all[m.shown[m.cur]], true
}

// reload relê a pasta; keep é o caminho que deve continuar selecionado.
func (m *model) reload(keep string) {
	items, err := scan(m.dir)
	if err != nil {
		m.err = err.Error()
	}
	m.all = items
	for p := range m.sel {
		if _, err := os.Stat(p); err != nil {
			delete(m.sel, p)
		}
	}
	if keep == "" {
		if it, ok := m.current(); ok {
			keep = it.path
		}
	}
	m.applyFilter(keep)
}

func (m *model) applyFilter(keep string) {
	words := strings.Fields(strings.ToLower(m.filter.Value()))
	m.shown = m.shown[:0]
	for i, it := range m.all {
		name := strings.ToLower(it.name)
		ok := true
		for _, w := range words {
			if !strings.Contains(name, w) {
				ok = false
				break
			}
		}
		if ok {
			m.shown = append(m.shown, i)
		}
	}
	m.cur = 0
	for i, idx := range m.shown {
		if m.all[idx].path == keep {
			m.cur = i
			break
		}
	}
	m.clamp()
}

func (m *model) clamp() {
	m.cur = max(0, min(m.cur, len(m.shown)-1))
	_, bodyH, _ := m.layout()
	if m.cur < m.off {
		m.off = m.cur
	}
	if m.cur >= m.off+bodyH {
		m.off = m.cur - bodyH + 1
	}
	m.off = max(0, min(m.off, max(0, len(m.shown)-bodyH)))
}

func (m *model) move(d int) { m.cur += d; m.clamp() }

// targets são os arquivos marcados, ou o atual se nada estiver marcado.
func (m *model) targets() []string {
	var out []string
	for _, it := range m.all {
		if m.sel[it.path] {
			out = append(out, it.path)
		}
	}
	if len(out) == 0 {
		if it, ok := m.current(); ok {
			out = append(out, it.path)
		}
	}
	return out
}

// ── layout ──────────────────────────────────────────────────────────────────

// layout: largura da lista, altura do corpo e área da imagem no preview.
func (m *model) layout() (lw, bodyH int, prev rect) {
	bodyH = max(m.h-3, 1) // cabeçalho + separador + rodapé
	lw = m.w
	if m.preview {
		lw = max(28, min(m.w*40/100, m.w-20))
		prev = rect{col: lw + 3, row: 5, cols: max(m.w-lw-3, 1), rows: max(bodyH-3, 1)}
	}
	return
}

// want diz qual imagem deve aparecer agora (chave vazia = nenhuma).
func (m *model) want() (string, string, rect) {
	it, ok := m.current()
	if !ok {
		return "", "", rect{}
	}
	switch {
	case m.mode == modeFull:
		a := rect{1, 1, m.w, max(m.h-1, 1)}
		return fmt.Sprintf("%s|%v", it.path, a), it.path, a
	case m.mode == modeList && m.preview:
		_, _, a := m.layout()
		return fmt.Sprintf("%s|%v", it.path, a), it.path, a
	}
	return "", "", rect{}
}

// ── update ──────────────────────────────────────────────────────────────────

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)

	// sincroniza a imagem do kitty com o estado
	key, _, _ := m.want()
	if key != m.wantKey {
		m.wantKey = key
		m.seq++
		if key == "" {
			clearImage()
		} else {
			seq := m.seq
			cmd = tea.Batch(cmd, tea.Tick(60*time.Millisecond, func(time.Time) tea.Msg { return showMsg{seq} }))
		}
	}
	return m, cmd
}

func (m *model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.clamp()
		return nil
	case showMsg:
		if msg.seq != m.seq {
			return nil
		}
		_, path, area := m.want()
		seq := msg.seq
		return func() tea.Msg {
			esc, _ := buildImage(path, area)
			return imgMsg{seq, esc}
		}
	case imgMsg:
		if msg.seq != m.seq {
			return nil
		}
		if msg.esc == "" {
			clearImage()
		} else {
			tty.WriteString(msg.esc)
		}
		return nil
	case statusMsg:
		m.statusID++
		m.status = msg.text
		id := m.statusID
		return tea.Tick(2500*time.Millisecond, func(time.Time) tea.Msg { return clearStatusMsg{id} })
	case clearStatusMsg:
		if msg.id == m.statusID {
			m.status = ""
		}
		return nil
	case tea.KeyMsg:
		switch m.mode {
		case modeFull:
			m.mode = modeList
			return nil
		case modeRename:
			return m.updateRename(msg)
		case modeTrash:
			return m.updateTrash(msg)
		}
		return m.updateList(msg)
	}
	// blink do cursor etc.
	var cmd tea.Cmd
	switch {
	case m.mode == modeRename:
		m.input, cmd = m.input.Update(msg)
	case m.searching:
		m.filter, cmd = m.filter.Update(msg)
	}
	return cmd
}

func (m *model) flash(text string) tea.Cmd {
	return func() tea.Msg { return statusMsg{text} }
}

func (m *model) updateList(msg tea.KeyMsg) tea.Cmd {
	_, bodyH, _ := m.layout()
	key := msg.String()

	// teclas que valem com ou sem a busca aberta
	switch key {
	case "up":
		m.move(-1)
		return nil
	case "down":
		m.move(1)
		return nil
	case "pgup":
		m.move(-bodyH)
		return nil
	case "pgdown":
		m.move(bodyH)
		return nil
	case "enter":
		if it, ok := m.current(); ok {
			if err := openFile(it.path); err != nil {
				return m.flash("erro: " + err.Error())
			}
		}
		return nil
	case "tab":
		if it, ok := m.current(); ok {
			if m.sel[it.path] {
				delete(m.sel, it.path)
			} else {
				m.sel[it.path] = true
			}
			m.move(1)
		}
		return nil
	case "ctrl+f":
		if _, ok := m.current(); ok {
			m.mode = modeFull
		}
		return nil
	case "ctrl+y":
		it, ok := m.current()
		if !ok {
			return nil
		}
		return func() tea.Msg {
			if err := copyImage(it.path); err != nil {
				return statusMsg{"erro ao copiar: " + err.Error()}
			}
			return statusMsg{"copiado: " + it.name}
		}
	case "f2":
		if it, ok := m.current(); ok {
			m.input.SetValue(strings.TrimSuffix(it.name, filepath.Ext(it.name)))
			m.input.CursorEnd()
			m.err = ""
			m.mode = modeRename
			return m.input.Focus()
		}
		return nil
	case "ctrl+d":
		if len(m.targets()) > 0 {
			m.mode = modeTrash
		}
		return nil
	case "ctrl+p":
		m.preview = !m.preview
		m.clamp()
		return nil
	case "ctrl+c":
		return m.quit()
	}

	if m.searching {
		if key == "esc" {
			m.searching = false
			m.filter.Blur()
			m.filter.SetValue("")
			it, _ := m.current()
			m.applyFilter(it.path)
			return nil
		}
		prev := m.filter.Value()
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		if m.filter.Value() != prev {
			m.applyFilter("")
		}
		return cmd
	}

	switch key {
	case "q":
		return m.quit()
	case "/":
		m.searching = true
		return m.filter.Focus()
	case "k":
		m.move(-1)
	case "j":
		m.move(1)
	case "home", "g":
		m.cur = 0
		m.clamp()
	case "end", "G":
		m.cur = len(m.shown) - 1
		m.clamp()
	}
	return nil
}

func (m *model) quit() tea.Cmd {
	clearImage()
	return tea.Quit
}

func (m *model) updateRename(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeList
		return nil
	case "enter":
		it, _ := m.current()
		dst, err := renameFile(it.path, m.input.Value())
		if err != nil {
			m.err = err.Error()
			return nil
		}
		m.mode = modeList
		m.reload(dst)
		return nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.err = ""
	return cmd
}

func (m *model) updateTrash(msg tea.KeyMsg) tea.Cmd {
	m.mode = modeList
	switch msg.String() {
	case "y", "Y", "s", "S", "enter":
		n := len(m.targets())
		if err := trash(m.targets()); err != nil {
			return m.flash("erro: " + err.Error())
		}
		m.reload("")
		return m.flash(fmt.Sprintf("%d arquivo(s) na lixeira", n))
	}
	return nil
}

// ── view ────────────────────────────────────────────────────────────────────

func fit(s string, w int, pad lipgloss.Style) string {
	s = ansi.Truncate(s, w, "…")
	if d := w - lipgloss.Width(s); d > 0 {
		s += pad.Render(strings.Repeat(" ", d))
	}
	return s
}

func fg(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

func (m *model) View() string {
	if m.w < 40 || m.h < 8 {
		return "terminal pequeno demais"
	}
	switch m.mode {
	case modeRename:
		return m.modal("renomear", m.input.View(), m.err)
	case modeTrash:
		return m.modal("lixeira", fmt.Sprintf("Mover %d arquivo(s) para a lixeira?", len(m.targets())),
			"[y] sim   [qualquer outra] cancelar")
	case modeFull:
		return m.viewFull()
	}

	lw, bodyH, _ := m.layout()
	plain := lipgloss.NewStyle()
	lines := make([]string, 0, m.h)

	// cabeçalho
	if m.searching {
		lines = append(lines, m.filter.View())
	} else {
		head := fg(cMuted).Render(m.dir)
		count := fmt.Sprintf("%d imagens", len(m.shown))
		if n := len(m.sel); n > 0 {
			count += fmt.Sprintf(" · %d marcadas", n)
		}
		head += "  " + fg(cMuted).Render(count)
		lines = append(lines, head)
	}

	// corpo: lista | preview
	pl := m.previewLines(bodyH)
	for i := 0; i < bodyH; i++ {
		row := fit(m.listRow(m.off+i, lw), lw, plain)
		if m.preview {
			row += fg(cBorder).Render("│") + pl[i]
		}
		lines = append(lines, row)
	}

	// rodapé
	lines = append(lines, fg(cBorder).Render(strings.Repeat("─", m.w)))
	lines = append(lines, m.footer())
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, m.w, "")
	}
	return strings.Join(lines, "\n")
}

func (m *model) listRow(i, w int) string {
	if i >= len(m.shown) {
		return ""
	}
	it := m.all[m.shown[i]]
	st := lipgloss.NewStyle()
	cursor := i == m.cur
	if cursor {
		st = st.Background(cSurface)
	}
	bar, mark := " ", " "
	if cursor {
		bar = "▌"
	}
	if m.sel[it.path] {
		mark = "●"
	}
	row := st.Foreground(cAccent).Render(bar+mark) + st.Render(" ") +
		st.Foreground(cMuted).Render(it.mod.Format("2006-01-02 15:04")) + st.Render("  ") +
		st.Foreground(cText).Render(it.name)
	if cursor {
		return fit(row, w, st)
	}
	return row
}

func (m *model) previewLines(bodyH int) []string {
	out := make([]string, bodyH)
	if !m.preview {
		return out
	}
	_, _, area := m.layout()
	pw := area.cols
	if it, ok := m.current(); ok {
		out[0] = " " + fit(fg(cStrong).Render(it.name), pw, lipgloss.NewStyle())
		out[1] = " " + fit(fg(cMuted).Render(m.info(it)), pw, lipgloss.NewStyle())
	} else {
		out[0] = " " + fg(cMuted).Render("nenhuma imagem")
	}
	return out
}

func (m *model) info(it item) string {
	if s, ok := m.dims[it.path]; ok {
		return s
	}
	s := humanSize(it.size)
	if w, h, ok := imageDims(it.path); ok {
		s = fmt.Sprintf("%dx%d · %s", w, h, s)
	}
	s += " · " + it.mod.Format("02/01/2006 15:04")
	m.dims[it.path] = s
	return s
}

func (m *model) footer() string {
	if m.status != "" {
		return fg(cAccent).Render(m.status)
	}
	keys := [][2]string{{"C-f", "tela-cheia"}, {"C-y", "copiar"}, {"F2", "renomear"}, {"C-d", "lixeira"},
		{"tab", "marcar"}, {"C-p", "preview"}, {"/", "buscar"}, {"q", "sair"}}
	if m.searching {
		keys[len(keys)-1] = [2]string{"esc", "fechar busca"}
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fg(cAccent).Render(k[0]) + " " + fg(cMuted).Render(k[1])
	}
	return strings.Join(parts, "  ")
}

func (m *model) viewFull() string {
	lines := make([]string, m.h)
	if it, ok := m.current(); ok {
		lines[m.h-1] = fg(cMuted).Render(it.name + " · " + m.info(it) + "  —  qualquer tecla para voltar")
	}
	return strings.Join(lines, "\n")
}

// modal desenha uma caixa centralizada no lugar da tela.
func (m *model) modal(title, body, hint string) string {
	w := min(60, m.w-4)
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(cAccent).Padding(0, 1).Width(w - 2)
	content := fg(cAccent).Render(title) + "\n\n" + body
	if hint != "" {
		content += "\n\n" + fg(cMuted).Render(hint)
	}
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box.Render(content))
}

func main() {
	dir := os.Getenv("SHOTS_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "Pictures", "Screenshots")
	}
	if len(os.Args) > 1 {
		if os.Args[1] == "-h" || os.Args[1] == "--help" {
			fmt.Println("uso: shots [diretório]")
			return
		}
		dir = os.Args[1]
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, "shots: diretório inválido: %s\n", dir)
		os.Exit(1)
	}
	if _, err := tea.NewProgram(newModel(dir), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "shots:", err)
		os.Exit(1)
	}
}
