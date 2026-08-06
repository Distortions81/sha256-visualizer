package main

import (
	"fmt"
	"image/color"
	"log"
	"math/bits"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	screenW = 1600
	screenH = 980

	bitSize   = 18
	bitGap    = 2
	rowGap    = 14
	leftPad   = 120
	topPad    = 80
	blockGap  = 70
	labelPad  = 80
	maxRounds = 64
)

var k = [64]uint32{
	0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5,
	0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
	0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3,
	0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
	0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc,
	0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
	0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7,
	0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
	0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13,
	0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
	0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3,
	0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
	0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5,
	0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
	0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208,
	0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
}

type Reg int

const (
	RegA Reg = iota
	RegB
	RegC
	RegD
	RegE
	RegF
	RegG
	RegH
	RegW
)

var regNames = []string{"a", "b", "c", "d", "e", "f", "g", "h", "w0"}

type State struct {
	a, b, c, d, e, f, g, h uint32
}

type Snapshot struct {
	Round int
	S     State
	T1    uint32
	T2    uint32
	W     uint32
	K     uint32
}

type Game struct {
	base       [maxRounds + 1]Snapshot
	mut        [maxRounds + 1]Snapshot
	showRound  int
	auto       bool
	autoFrames int

	selReg Reg
	selBit int // 0 = MSB, 31 = LSB visually

	messageWord uint32
	init        State
}

func ch(x, y, z uint32) uint32  { return (x & y) ^ (^x & z) }
func maj(x, y, z uint32) uint32 { return (x & y) ^ (x & z) ^ (y & z) }

func bigSigma0(x uint32) uint32 {
	return bits.RotateLeft32(x, -2) ^ bits.RotateLeft32(x, -13) ^ bits.RotateLeft32(x, -22)
}

func bigSigma1(x uint32) uint32 {
	return bits.RotateLeft32(x, -6) ^ bits.RotateLeft32(x, -11) ^ bits.RotateLeft32(x, -25)
}

func defaultState() State {
	return State{
		a: 0x6a09e667,
		b: 0xbb67ae85,
		c: 0x3c6ef372,
		d: 0xa54ff53a,
		e: 0x510e527f,
		f: 0x9b05688c,
		g: 0x1f83d9ab,
		h: 0x5be0cd19,
	}
}

func newGame() *Game {
	g := &Game{
		init:        defaultState(),
		messageWord: 0x61626380, // first word for padded "abc"
		selReg:      RegA,
		selBit:      0,
	}
	g.recompute()
	return g
}

func (g *Game) recompute() {
	g.base = runTrace(g.init, g.messageWord, g.selReg, -1)
	g.mut = runTrace(g.init, g.messageWord, g.selReg, g.selBit)
	if g.showRound > maxRounds {
		g.showRound = maxRounds
	}
}

func runTrace(init State, w0 uint32, selReg Reg, selBit int) [maxRounds + 1]Snapshot {
	var out [maxRounds + 1]Snapshot
	s := init
	w := w0
	if selBit >= 0 {
		mask := uint32(1) << (31 - selBit)
		switch selReg {
		case RegA:
			s.a ^= mask
		case RegB:
			s.b ^= mask
		case RegC:
			s.c ^= mask
		case RegD:
			s.d ^= mask
		case RegE:
			s.e ^= mask
		case RegF:
			s.f ^= mask
		case RegG:
			s.g ^= mask
		case RegH:
			s.h ^= mask
		case RegW:
			w ^= mask
		}
	}

	out[0] = Snapshot{Round: 0, S: s, W: w, K: k[0]}
	for i := 0; i < maxRounds; i++ {
		wi := uint32(0)
		if i == 0 {
			wi = w
		}
		t1 := s.h + bigSigma1(s.e) + ch(s.e, s.f, s.g) + k[i] + wi
		t2 := bigSigma0(s.a) + maj(s.a, s.b, s.c)
		next := State{
			a: t1 + t2,
			b: s.a,
			c: s.b,
			d: s.c,
			e: s.d + t1,
			f: s.e,
			g: s.f,
			h: s.g,
		}
		s = next
		wNext := uint32(0)
		if i+1 < maxRounds {
			wNext = 0 // keep only W0 non-zero for a focused visualization
		}
		out[i+1] = Snapshot{Round: i + 1, S: s, T1: t1, T2: t2, W: wNext, K: k[min(i+1, maxRounds-1)]}
	}
	return out
}

func (g *Game) Update() error {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.auto = !g.auto
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.showRound = 0
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketRight) || inpututil.IsKeyJustPressed(ebiten.KeyN) {
		if g.showRound < maxRounds {
			g.showRound++
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft) || inpututil.IsKeyJustPressed(ebiten.KeyP) {
		if g.showRound > 0 {
			g.showRound--
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		g.selBit--
		if g.selBit < 0 {
			g.selBit = 31
		}
		g.recompute()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
		g.selBit++
		if g.selBit > 31 {
			g.selBit = 0
		}
		g.recompute()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) {
		g.selReg--
		if g.selReg < 0 {
			g.selReg = RegW
		}
		g.recompute()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyRight) {
		g.selReg++
		if g.selReg > RegW {
			g.selReg = RegA
		}
		g.recompute()
	}

	if inpututil.IsKeyJustPressed(ebiten.Key1) {
		g.selReg = RegA
		g.recompute()
	}
	if inpututil.IsKeyJustPressed(ebiten.Key2) {
		g.selReg = RegB
		g.recompute()
	}
	if inpututil.IsKeyJustPressed(ebiten.Key3) {
		g.selReg = RegC
		g.recompute()
	}
	if inpututil.IsKeyJustPressed(ebiten.Key4) {
		g.selReg = RegD
		g.recompute()
	}
	if inpututil.IsKeyJustPressed(ebiten.Key5) {
		g.selReg = RegE
		g.recompute()
	}
	if inpututil.IsKeyJustPressed(ebiten.Key6) {
		g.selReg = RegF
		g.recompute()
	}
	if inpututil.IsKeyJustPressed(ebiten.Key7) {
		g.selReg = RegG
		g.recompute()
	}
	if inpututil.IsKeyJustPressed(ebiten.Key8) {
		g.selReg = RegH
		g.recompute()
	}
	if inpututil.IsKeyJustPressed(ebiten.Key9) {
		g.selReg = RegW
		g.recompute()
	}

	if g.auto {
		g.autoFrames++
		if g.autoFrames >= 20 {
			g.autoFrames = 0
			g.showRound++
			if g.showRound > maxRounds {
				g.showRound = 0
			}
		}
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{14, 18, 24, 255})

	curBase := g.base[g.showRound]
	curMut := g.mut[g.showRound]
	curDiff := diffState(curBase.S, curMut.S)

	drawTitle(screen, fmt.Sprintf("SHA-256 differential bit visualizer   round=%d   source=%s[%d]", g.showRound, regNames[g.selReg], g.selBit))
	info := "Arrows: choose source register/bit   [ / ] or P/N: round   Space: autoplay   R: reset round   1..9: a..h,w0\nHighlighted cells show which current bits change when exactly one chosen source bit is toggled. This is influence, not one-to-one movement."
	ebitenutil.DebugPrintAt(screen, info, 24, 34)

	x0 := float32(leftPad)
	y0 := float32(topPad)
	drawStateBlock(screen, x0, y0, "baseline state", curBase.S, zeroState(), g.selReg, g.selBit, false)
	drawStateBlock(screen, x0, y0+float32(8*(bitSize+rowGap)+blockGap), "influenced bits (baseline XOR mutated)", curDiff, curDiff, g.selReg, g.selBit, true)

	x1 := x0 + float32(32*(bitSize+bitGap)) + 180
	drawWord(screen, x1, y0, "W0", curBase.W, selectedSourceMask(g.selReg, g.selBit, RegW), g.selReg == RegW)
	drawWord(screen, x1, y0+90, "K[i]", curBase.K, 0, false)
	drawWord(screen, x1, y0+180, "T1", curBase.T1, curBase.T1^curMut.T1, true)
	drawWord(screen, x1, y0+270, "T2", curBase.T2, curBase.T2^curMut.T2, true)

	drawLegend(screen, x1, y0+380)
}

func drawTitle(screen *ebiten.Image, s string) {
	ebitenutil.DebugPrintAt(screen, s, 24, 12)
}

func zeroState() State { return State{} }

func diffState(a, b State) State {
	return State{
		a: a.a ^ b.a,
		b: a.b ^ b.b,
		c: a.c ^ b.c,
		d: a.d ^ b.d,
		e: a.e ^ b.e,
		f: a.f ^ b.f,
		g: a.g ^ b.g,
		h: a.h ^ b.h,
	}
}

func selectedSourceMask(sel Reg, bit int, row Reg) uint32 {
	if sel != row {
		return 0
	}
	return uint32(1) << (31 - bit)
}

func drawStateBlock(screen *ebiten.Image, x, y float32, title string, s State, diff State, sel Reg, selBit int, diffMode bool) {
	ebitenutil.DebugPrintAt(screen, title, int(x), int(y)-24)
	rows := []struct {
		name string
		v    uint32
		d    uint32
		reg  Reg
	}{
		{"a", s.a, diff.a, RegA},
		{"b", s.b, diff.b, RegB},
		{"c", s.c, diff.c, RegC},
		{"d", s.d, diff.d, RegD},
		{"e", s.e, diff.e, RegE},
		{"f", s.f, diff.f, RegF},
		{"g", s.g, diff.g, RegG},
		{"h", s.h, diff.h, RegH},
	}
	for i, row := range rows {
		ry := y + float32(i*(bitSize+rowGap))
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s  %08x", row.name, row.v), int(x)-labelPad, int(ry)+2)
		srcMask := selectedSourceMask(sel, selBit, row.reg)
		drawBits(screen, x, ry, row.v, row.d, srcMask, diffMode)
	}
}

func drawWord(screen *ebiten.Image, x, y float32, label string, v uint32, diffMask uint32, diffMode bool) {
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s  %08x", label, v), int(x), int(y)-24)
	drawBits(screen, x, y, v, diffMask, 0, diffMode)
}

func drawBits(screen *ebiten.Image, x, y float32, v uint32, diffMask uint32, sourceMask uint32, diffMode bool) {
	for i := 0; i < 32; i++ {
		bitMask := uint32(1) << (31 - i)
		bx := x + float32(i*(bitSize+bitGap))
		baseCol := color.RGBA{45, 56, 72, 255}
		if v&bitMask != 0 {
			baseCol = color.RGBA{92, 113, 148, 255}
		}
		if diffMode && diffMask&bitMask != 0 {
			baseCol = color.RGBA{240, 182, 65, 255}
		}
		if sourceMask&bitMask != 0 {
			baseCol = color.RGBA{70, 220, 120, 255}
		}
		drawFilledRect(screen, bx, y, bitSize, bitSize, baseCol)
		if i%4 == 0 {
			vector.StrokeLine(screen, bx-1, y-3, bx-1, y+bitSize+3, 1, color.RGBA{110, 120, 140, 255}, false)
		}
	}
}

func drawLegend(screen *ebiten.Image, x, y float32) {
	items := []struct {
		name string
		col  color.RGBA
	}{
		{"0 bit", color.RGBA{45, 56, 72, 255}},
		{"1 bit", color.RGBA{92, 113, 148, 255}},
		{"selected source bit", color.RGBA{70, 220, 120, 255}},
		{"bit changed due to source bit", color.RGBA{240, 182, 65, 255}},
	}
	for i, it := range items {
		ty := y + float32(i*28)
		drawFilledRect(screen, x, ty, 22, 22, it.col)
		ebitenutil.DebugPrintAt(screen, it.name, int(x)+32, int(ty)+3)
	}
	lines := []string{
		"Interpretation:",
		"- left panel is the real state at the chosen round.",
		"- lower panel is a differential view: baseline XOR mutated.",
		"- yellow means toggling one chosen source bit flips this current bit.",
		"- SHA-256 does not move bits like a wire; rotations, XOR, AND, and addition cause fan-out and nonlinear mixing.",
		"- this sample keeps only W0 non-zero so the first-wave influence is easier to see.",
	}
	for i, s := range lines {
		ebitenutil.DebugPrintAt(screen, s, int(x), int(y)+140+i*18)
	}
}

func drawFilledRect(screen *ebiten.Image, x, y, w, h float32, c color.Color) {
	vector.DrawFilledRect(screen, x, y, w, h, c, false)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenW, screenH
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	ebiten.SetWindowSize(screenW, screenH)
	ebiten.SetWindowTitle("SHA-256 bit visualizer")
	if err := ebiten.RunGame(newGame()); err != nil && err != ebiten.Termination {
		log.Fatal(err)
	}
}
