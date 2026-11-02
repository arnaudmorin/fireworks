package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Overridable at build time: -ldflags "-X main.maxDuration=10s -X 'main.message=Hello'"
var (
	maxDuration = ""
	message     = "Merci Emma!"
)

const (
	fps          = 25
	dt           = 1.0 / fps
	persistence  = 0.60
	maxParticles = 9000
	finaleSpan   = 3.2
	hintText     = "  espace: salve   f: finale   c: couleurs   q: quitter  "
)

type color struct{ r, g, b float64 }

func (c color) mul(k float64) color { return color{c.r * k, c.g * k, c.b * k} }

func mix(a, b color, t float64) color {
	return color{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

var (
	white  = color{1, 1, 1}
	gold   = color{1, 0.72, 0.28}
	ember  = color{1, 0.45, 0.12}
	silver = color{0.92, 0.93, 1}
)

var palette = []color{
	{1, 0.12, 0.08},
	{1, 0.42, 0.05},
	{1, 0.78, 0.25},
	{0.15, 1, 0.3},
	{0.2, 0.55, 1},
	{0.65, 0.25, 1},
	{1, 0.25, 0.65},
	{0.25, 1, 0.95},
	silver,
}

const (
	kPeony = iota
	kChrys
	kWillow
	kRing
	kCrossette
	kPalm
	kGlitter
	kPistil
	kCrackle
	kSaturn
	kHeart
	kSmiley
	kBrocade
	kText
	kCount
)

var kindWeights = [kCount]float64{3, 3, 2, 2, 1.6, 1.4, 1.4, 2, 1.5, 1, 0.6, 0.4, 0.8, 0}

var textColors = []color{gold, {1, 0.3, 0.6}, {0.3, 0.9, 1}, {0.5, 1, 0.4}}

type particle struct {
	x, y, px, py  float64
	vx, vy        float64
	life, maxLife float64
	c, trailCol   color
	drag, grav    float64
	trail         float64
	bright        float64
	hot           float64
	twinkle       bool
	crackle       bool
	split         int
	splitAt       float64
	splitV        float64
}

type rocket struct {
	x, y, vx, vy float64
	kind         int
	c1, c2       color
}

type pending struct {
	at     float64
	x      float64
	kind   int
	c1, c2 color
}

type star struct {
	i   int
	b   float32
	dim int
}

type window struct {
	i         int
	on        bool
	lit, dark color
}

type world struct {
	w, h       int
	rng        *rand.Rand
	t          float64
	g, scale   float64
	light      []float32
	bg         []color
	solid      []bool
	starB      []float32
	stars      []star
	windows    []window
	beacons    []int
	parts      []particle
	spawn      []particle
	rockets    []rocket
	queue      []pending
	nextFinale float64
	quietUntil float64
}

func newWorld(w, h int) *world {
	wd := &world{rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
	wd.resize(w, h)
	wd.nextFinale = 70 + wd.rng.Float64()*30
	wd.queue = append(wd.queue, pending{at: 0.3, x: float64(w) / 2, kind: kChrys, c1: palette[0], c2: palette[2]})
	return wd
}

func (w *world) resize(cols, h int) {
	w.w, w.h = cols, h
	w.g = float64(h) * 0.5
	w.scale = math.Min(float64(h), float64(cols)*0.7)
	w.light = make([]float32, cols*h*3)
	w.buildScenery()
}

func (w *world) frand(a, b float64) float64 { return a + w.rng.Float64()*(b-a) }

func (w *world) buildScenery() {
	W, H, rng := w.w, w.h, w.rng
	n := W * H
	w.bg = make([]color, n)
	w.solid = make([]bool, n)
	w.starB = make([]float32, n)
	w.stars, w.windows, w.beacons = nil, nil, nil

	top := color{3.0 / 255, 3.0 / 255, 14.0 / 255}
	horizon := color{28.0 / 255, 16.0 / 255, 48.0 / 255}
	mx, my := float64(W)*0.83, float64(H)*0.16
	mr := math.Max(2.5, float64(H)*0.045)
	moon := color{0.93, 0.9, 0.78}
	for y := 0; y < H; y++ {
		sky := mix(top, horizon, math.Pow(float64(y)/float64(H), 1.6))
		for x := 0; x < W; x++ {
			c := sky
			fx, fy := float64(x)+0.5, float64(y)+0.5
			d := math.Hypot(fx-mx, fy-my)
			if d < mr*4 {
				k := 1 - d/(mr*4)
				c = mix(c, color{0.25, 0.25, 0.35}, k*k*0.35)
			}
			if d < mr && math.Hypot(fx-(mx+mr*0.55), fy-(my-mr*0.25)) > mr*0.92 {
				c = moon
			}
			w.bg[y*W+x] = c
		}
	}

	backCol := color{0.075, 0.06, 0.13}
	frontCol := color{0.025, 0.022, 0.05}
	fill := func(x0, x1, y0 int, c color) {
		for y := clampI(y0, 0, H); y < H; y++ {
			for x := clampI(x0, 0, W); x < clampI(x1, 0, W); x++ {
				w.bg[y*W+x] = c
				w.solid[y*W+x] = true
			}
		}
	}
	addWindows := func(x0, x1, y0 int, base, lit color, p float64) {
		for y := y0 + 2; y < H-1; y += 3 {
			for x := x0 + 1; x < x1-1; x += 2 {
				if x < 0 || x >= W || y < 0 {
					continue
				}
				win := window{i: y*W + x, on: rng.Float64() < p, lit: lit.mul(w.frand(0.6, 1)), dark: base}
				if win.on {
					w.bg[win.i] = win.lit
				}
				w.windows = append(w.windows, win)
			}
		}
	}
	for x := 0; x < W; {
		bw := 4 + rng.Intn(9)
		y0 := H - int(float64(H)*w.frand(0.12, 0.28))
		fill(x, x+bw, y0, backCol)
		addWindows(x, x+bw, y0, backCol, color{0.45, 0.35, 0.16}, 0.25)
		x += bw
	}
	for x := -rng.Intn(3); x < W; {
		bw := 3 + rng.Intn(8)
		y0 := H - int(float64(H)*w.frand(0.05, 0.16))
		fill(x, x+bw, y0, frontCol)
		addWindows(x, x+bw, y0, frontCol, color{0.7, 0.55, 0.22}, 0.35)
		if rng.Float64() < 0.15 && bw > 3 {
			ax := x + bw/2
			ah := 2 + rng.Intn(4)
			for y := y0 - ah; y < y0; y++ {
				if ax >= 0 && ax < W && y >= 0 {
					w.bg[y*W+ax] = frontCol
					w.solid[y*W+ax] = true
				}
			}
			if ax >= 0 && ax < W && y0-ah-1 >= 0 {
				i := (y0-ah-1)*W + ax
				w.solid[i] = true
				w.beacons = append(w.beacons, i)
			}
		}
		x += bw + rng.Intn(2)
	}

	for k := 0; k < W*H/90; k++ {
		x, y := rng.Intn(W), rng.Intn(int(float64(H)*0.7)+1)
		if y >= H {
			continue
		}
		i := y*W + x
		if w.solid[i] || math.Hypot(float64(x)-mx, float64(y)-my) < mr*2 {
			continue
		}
		b := float32(w.frand(0.15, 0.7))
		w.stars = append(w.stars, star{i: i, b: b})
		w.starB[i] = b
	}
}

func clampI(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (w *world) add(x, y float64, c color, a float64) {
	if x < 0 || y < 0 {
		return
	}
	ix, iy := int(x), int(y)
	if ix >= w.w || iy >= w.h {
		return
	}
	j := (iy*w.w + ix) * 3
	w.light[j] += float32(c.r * a)
	w.light[j+1] += float32(c.g * a)
	w.light[j+2] += float32(c.b * a)
}

func (w *world) line(x0, y0, x1, y1 float64, c color, a float64) {
	steps := int(math.Ceil(math.Max(math.Abs(x1-x0), math.Abs(y1-y0))))
	if steps < 1 {
		w.add(x1, y1, c, a)
		return
	}
	for s := 1; s <= steps; s++ {
		t := float64(s) / float64(steps)
		w.add(x0+(x1-x0)*t, y0+(y1-y0)*t, c, a*(0.45+0.55*t))
	}
}

func (w *world) glow(cx, cy float64, c color, r, a float64) {
	for y := int(cy - r); y <= int(cy+r); y++ {
		for x := int(cx - r); x <= int(cx+r); x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			if d < r {
				k := 1 - d/r
				w.add(float64(x), float64(y), c, a*k*k)
			}
		}
	}
}

func (w *world) emit(p particle) {
	if len(w.parts)+len(w.spawn) >= maxParticles {
		return
	}
	p.px, p.py = p.x, p.y
	w.spawn = append(w.spawn, p)
}

func (w *world) shot(x, y, dx, dy, R float64, tpl particle) {
	p := tpl
	p.x, p.y = x, y
	p.vx, p.vy = dx*R*tpl.drag, dy*R*tpl.drag
	p.maxLife = tpl.maxLife * w.frand(0.85, 1.15)
	p.life = p.maxLife
	w.emit(p)
}

func (w *world) sphere(x, y float64, n int, R float64, tpl particle) {
	for i := 0; i < n; i++ {
		z := w.frand(-1, 1)
		ph := w.frand(0, 2*math.Pi)
		s := math.Sqrt(1-z*z) * w.frand(0.9, 1)
		w.shot(x, y, s*math.Cos(ph), s*math.Sin(ph), R, tpl)
	}
}

func (w *world) ring(x, y float64, n int, R, tilt float64, tpl particle) {
	rot := w.frand(0, math.Pi)
	cr, sr := math.Cos(rot), math.Sin(rot)
	for i := 0; i < n; i++ {
		a := 2*math.Pi*float64(i)/float64(n) + w.frand(-0.03, 0.03)
		dx, dy := math.Cos(a), math.Sin(a)*tilt
		w.shot(x, y, dx*cr-dy*sr, dx*sr+dy*cr, R, tpl)
	}
}

func (w *world) shape(x, y float64, pts [][2]float64, R float64, tpl particle) {
	rot := w.frand(-0.3, 0.3)
	cr, sr := math.Cos(rot), math.Sin(rot)
	for _, p := range pts {
		w.shot(x, y, p[0]*cr-p[1]*sr, p[0]*sr+p[1]*cr, R, tpl)
	}
}

func jitter(c color, r *rand.Rand) color {
	f := func(v float64) float64 { return math.Min(1, v*(0.9+0.2*r.Float64())) }
	return color{f(c.r), f(c.g), f(c.b)}
}

func (w *world) randomShell() (int, color, color) {
	total := 0.0
	for _, v := range kindWeights {
		total += v
	}
	r := w.rng.Float64() * total
	kind := 0
	for k, v := range kindWeights {
		if r < v {
			kind = k
			break
		}
		r -= v
	}
	i := w.rng.Intn(len(palette))
	j := (i + 1 + w.rng.Intn(len(palette)-1)) % len(palette)
	return kind, jitter(palette[i], w.rng), jitter(palette[j], w.rng)
}

func (w *world) burst(x, y float64, kind int, c1, c2 color) {
	R := w.scale * w.frand(0.16, 0.28)
	n := int(60 + w.scale*1.1)
	base := particle{c: c1, drag: 1.7, grav: 0.25, maxLife: 1.6, bright: 0.75, hot: 0.05}
	trailed := base
	trailed.trail = 28
	trailed.trailCol = mix(c1, gold, 0.5).mul(0.7)
	trailed.maxLife = 1.9

	w.glow(x, y, c1, R*0.8, 0.16)
	w.glow(x, y, white, 2.5, 0.7)

	switch kind {
	case kPeony:
		w.sphere(x, y, n, R, base)
	case kChrys:
		w.sphere(x, y, n, R, trailed)
	case kWillow, kBrocade:
		p := base
		p.c, p.trail, p.trailCol = gold, 30, gold.mul(0.55)
		p.drag, p.grav, p.maxLife, p.bright = 2.2, 0.45, 3.6, 0.7
		if kind == kBrocade {
			p.trail, p.drag, p.grav, p.maxLife = 45, 1.9, 0.4, 2.6
			R *= 1.15
		}
		w.sphere(x, y, n*4/5, R*1.05, p)
	case kRing:
		tilt := w.frand(0.25, 1)
		w.ring(x, y, n/2, R, tilt, base)
		if w.rng.Float64() < 0.4 {
			p := base
			p.c = c2
			w.ring(x, y, n/3, R*0.6, tilt, p)
		}
	case kCrossette:
		p := base
		p.drag, p.trail, p.trailCol = 1.5, 22, mix(c1, gold, 0.5).mul(0.6)
		p.split, p.splitAt, p.splitV = 4, 0.5, R*0.4*2
		m := 10 + w.rng.Intn(5)
		for i := 0; i < m; i++ {
			a := 2*math.Pi*float64(i)/float64(m) + w.frand(-0.15, 0.15)
			w.shot(x, y, math.Cos(a), math.Sin(a), R*0.8, p)
		}
	case kPalm:
		p := base
		p.drag, p.grav, p.maxLife, p.trail = 1.6, 0.45, 2.2, 40
		p.trailCol = mix(c1, gold, 0.6).mul(0.6)
		arms := 7 + w.rng.Intn(4)
		for i := 0; i < arms; i++ {
			a := 2*math.Pi*float64(i)/float64(arms) + w.frand(-0.2, 0.2)
			for k := 0; k < 5; k++ {
				s := w.frand(0.55, 1)
				w.shot(x, y, math.Cos(a)*s, math.Sin(a)*s, R, p)
			}
		}
	case kGlitter:
		p := base
		p.c = silver
		if w.rng.Float64() < 0.5 {
			p.c = gold
		}
		p.twinkle, p.maxLife, p.drag, p.grav, p.trail = true, 2.4, 1.8, 0.3, 10
		p.trailCol = white.mul(0.4)
		w.sphere(x, y, n*7/10, R, p)
	case kPistil:
		w.sphere(x, y, n*3/4, R, base)
		p := base
		p.c, p.maxLife = c2, 1.4
		w.sphere(x, y, n*2/5, R*0.5, p)
	case kCrackle:
		p := base
		p.c, p.crackle, p.maxLife, p.trail, p.trailCol = mix(c1, gold, 0.6), true, 1.3, 8, gold.mul(0.4)
		w.sphere(x, y, n*3/5, R*0.75, p)
	case kSaturn:
		w.sphere(x, y, n*3/5, R*0.55, base)
		p := base
		p.c = c2
		w.ring(x, y, n/2, R*1.1, 0.22, p)
	case kHeart:
		p := base
		p.c, p.grav, p.maxLife = color{1, 0.2, 0.45}, 0.1, 2.0
		pts := make([][2]float64, 0, 70)
		for i := 0; i < 70; i++ {
			t := 2 * math.Pi * float64(i) / 70
			s := math.Sin(t)
			pts = append(pts, [2]float64{16 * s * s * s / 17, -(13*math.Cos(t) - 5*math.Cos(2*t) - 2*math.Cos(3*t) - math.Cos(4*t)) / 17})
		}
		w.shape(x, y, pts, R, p)
	case kSmiley:
		p := base
		p.c, p.grav, p.maxLife = color{1, 0.85, 0.15}, 0.1, 2.0
		var pts [][2]float64
		for i := 0; i < 44; i++ {
			a := 2 * math.Pi * float64(i) / 44
			pts = append(pts, [2]float64{math.Cos(a), math.Sin(a)})
		}
		for _, ex := range []float64{-0.33, 0.33} {
			for i := 0; i < 5; i++ {
				a := 2 * math.Pi * float64(i) / 5
				pts = append(pts, [2]float64{ex + 0.06*math.Cos(a), -0.3 + 0.06*math.Sin(a)})
			}
		}
		for i := 0; i < 14; i++ {
			a := math.Pi * (0.2 + 0.6*float64(i)/13)
			pts = append(pts, [2]float64{0.58 * math.Cos(a), 0.58 * math.Sin(a)})
		}
		w.shape(x, y, pts, R, p)
	case kText:
		w.text(x, y, message)
	}
}

func (w *world) text(cx, cy float64, msg string) {
	maxCols := int((float64(w.w)*0.9/1.6 + 1) / 6)
	if maxCols < 1 {
		maxCols = 1
	}
	lines := wrapText(msg, maxCols)
	if len(lines) == 0 {
		return
	}
	widest := 0
	for _, l := range lines {
		if len(l) > widest {
			widest = len(l)
		}
	}
	rows := len(lines)*9 - 2
	s := math.Min(float64(w.w)*0.9/float64(widest*6-1), float64(w.h)*0.5/float64(rows))
	sub := int(math.Ceil(s))
	y0 := math.Max(1, cy-float64(rows)*s/2)
	word := 0
	for li, line := range lines {
		x0 := cx - float64(len(line)*6-1)*s/2
		c := textColors[word%len(textColors)]
		for i, r := range line {
			if r == ' ' {
				word++
				c = textColors[word%len(textColors)]
				continue
			}
			g, ok := font[r]
			if !ok {
				continue
			}
			gc := c
			if r == '♥' {
				gc = color{1, 0.15, 0.3}
			}
			for row := 0; row < 7; row++ {
				for col := 0; col < 5; col++ {
					if g[row][col] != '#' {
						continue
					}
					for sy := 0; sy < sub; sy++ {
						for sx := 0; sx < sub; sx++ {
							tx := x0 + (float64(i*6+col)+(float64(sx)+0.5)/float64(sub))*s
							ty := y0 + (float64(li*9+row)+(float64(sy)+0.5)/float64(sub))*s
							l := w.frand(4.2, 4.8)
							// appended directly so the message survives a saturated particle budget
							w.spawn = append(w.spawn, particle{
								x: cx, y: cy, px: cx, py: cy, vx: (tx - cx) * 3, vy: (ty - cy) * 3,
								drag: 3, grav: 0.02, life: l, maxLife: l, c: gc, bright: 0.55, hot: 0.3,
								trail: 0.8, trailCol: gc.mul(0.5),
							})
						}
					}
				}
			}
		}
		word++
	}
}

func (w *world) split(p *particle) {
	a0 := math.Atan2(p.vy, p.vx) + math.Pi/4
	c := p.c
	c.r, c.g, c.b = math.Min(1, c.r*1.2), math.Min(1, c.g*1.2), math.Min(1, c.b*1.2)
	for k := 0; k < p.split; k++ {
		a := a0 + 2*math.Pi*float64(k)/float64(p.split)
		w.emit(particle{
			x: p.x, y: p.y, vx: math.Cos(a) * p.splitV, vy: math.Sin(a) * p.splitV,
			life: 0.9, maxLife: 0.9, c: c, drag: 2, grav: 0.3, trail: 18,
			trailCol: p.trailCol, bright: 0.9, hot: 0.05,
		})
	}
	w.add(p.x, p.y, white, 1)
}

func (w *world) crackle(x, y float64) {
	m := 3 + w.rng.Intn(3)
	sp := w.scale * 0.15
	for k := 0; k < m; k++ {
		l := w.frand(0.06, 0.16)
		w.emit(particle{
			x: x, y: y, vx: w.frand(-sp, sp), vy: w.frand(-sp, sp),
			life: l, maxLife: l, c: white, drag: 4, bright: 1.2,
		})
	}
	w.add(x, y, white, 1)
}

func (w *world) launch(x float64, kind int, c1, c2 color) {
	H := float64(w.h)
	apex, vx := H*w.frand(0.1, 0.45), w.frand(-1, 1)*w.scale*0.07
	if kind == kText {
		apex, vx = H*0.3, 0
	}
	w.rockets = append(w.rockets, rocket{
		x: x, y: H, vx: vx,
		vy:   -math.Sqrt(2 * w.g * (H - apex)),
		kind: kind, c1: c1, c2: c2,
	})
}

func (w *world) volley() {
	kind, c1, c2 := w.randomShell()
	W := float64(w.w)
	r := w.rng.Float64()
	switch {
	case r < 0.6:
		w.launch(W*w.frand(0.1, 0.9), kind, c1, c2)
	case r < 0.85:
		x := W * w.frand(0.1, 0.4)
		w.launch(x, kind, c1, c2)
		w.launch(W-x, kind, c1, c2)
	default:
		w.salvo(3+w.rng.Intn(3), kind, c1, c2)
	}
}

func (w *world) salvo(m, kind int, c1, c2 color) {
	W := float64(w.w)
	for i := 0; i < m; i++ {
		x := W * (0.1 + 0.8*(float64(i)+0.5)/float64(m))
		w.queue = append(w.queue, pending{at: w.t + 0.15*float64(i), x: x, kind: kind, c1: c1, c2: c2})
	}
}

func (w *world) finale() {
	W := float64(w.w)
	at := w.t
	for at < w.t+finaleSpan {
		kind, c1, c2 := w.randomShell()
		if w.rng.Float64() < 0.3 {
			kind = kBrocade
		}
		w.queue = append(w.queue, pending{at: at, x: W * w.frand(0.08, 0.92), kind: kind, c1: c1, c2: c2})
		at += w.frand(0.07, 0.18)
	}
	textAt := w.t + finaleSpan + 1.8
	if strings.TrimSpace(message) != "" {
		w.queue = append(w.queue, pending{at: textAt, x: W / 2, kind: kText})
	}
	w.quietUntil = textAt + 6
	w.nextFinale = w.t + 80 + w.rng.Float64()*40
}

func (w *world) step() {
	w.t += dt
	rng := w.rng

	for i, v := range w.light {
		v *= persistence
		if v < 0.04 {
			v = 0
		}
		w.light[i] = v
	}

	for k := range w.stars {
		s := &w.stars[k]
		if s.dim > 0 {
			s.dim--
			if s.dim == 0 {
				w.starB[s.i] = s.b
			}
		} else if rng.Float64() < 0.002 {
			s.dim = 3 + rng.Intn(10)
			w.starB[s.i] = s.b * 0.2
		}
	}
	for k := range w.windows {
		win := &w.windows[k]
		if rng.Float64() < 0.0004 {
			win.on = !win.on
			if win.on {
				w.bg[win.i] = win.lit
			} else {
				w.bg[win.i] = win.dark
			}
		}
	}
	beacon := color{0.12, 0.02, 0.02}
	if math.Mod(w.t, 1.6) < 0.5 {
		beacon = color{1, 0.1, 0.05}
	}
	for _, i := range w.beacons {
		w.bg[i] = beacon
	}

	if w.t > w.nextFinale {
		w.finale()
	}
	rate := 0.45 + float64(w.w)/170
	if w.t > w.quietUntil && rng.Float64() < rate*dt {
		w.volley()
	}
	q := w.queue[:0]
	for _, p := range w.queue {
		if p.at <= w.t {
			w.launch(p.x, p.kind, p.c1, p.c2)
		} else {
			q = append(q, p)
		}
	}
	w.queue = q

	rk := w.rockets[:0]
	for _, r := range w.rockets {
		px, py := r.x, r.y
		r.vy += w.g * dt
		r.x += r.vx * dt
		r.y += r.vy * dt
		w.line(px, py, r.x, r.y, color{1, 0.85, 0.6}, 0.9)
		for k := 0; k < 2; k++ {
			l := w.frand(0.25, 0.5)
			w.emit(particle{
				x: r.x, y: r.y, vx: w.frand(-4, 4), vy: w.frand(4, 14),
				life: l, maxLife: l, c: ember, drag: 3, grav: 0.6, bright: 0.55,
			})
		}
		if r.vy > -w.g*0.12 {
			w.burst(r.x, r.y, r.kind, r.c1, r.c2)
			continue
		}
		rk = append(rk, r)
	}
	w.rockets = rk

	alive := w.parts[:0]
	for i := range w.parts {
		p := &w.parts[i]
		f := math.Exp(-p.drag * dt)
		p.px, p.py = p.x, p.y
		p.vx *= f
		p.vy = p.vy*f + w.g*p.grav*dt
		p.x += p.vx * dt
		p.y += p.vy * dt
		p.life -= dt
		if p.life <= 0 {
			if p.crackle {
				w.crackle(p.x, p.y)
			}
			continue
		}
		lf := p.life / p.maxLife
		if p.split > 0 && lf < p.splitAt {
			w.split(p)
			continue
		}
		br := p.bright
		if lf < 0.35 {
			br *= lf / 0.35
			if lf < 0.25 && rng.Float64() < 0.3 {
				br *= 0.25
			}
		}
		if p.twinkle {
			if rng.Float64() < 0.45 {
				br *= 0.08
			} else {
				br *= 1.3
			}
		}
		c := p.c
		if age := p.maxLife - p.life; age < p.hot {
			c = mix(white, c, age/p.hot)
		}
		w.line(p.px, p.py, p.x, p.y, c, br)
		for e := p.trail * dt; e > 0; e-- {
			if rng.Float64() < e {
				l := w.frand(0.3, 0.8)
				w.emit(particle{
					x: p.x, y: p.y, vx: p.vx*0.1 + w.frand(-3, 3), vy: p.vy*0.1 + w.frand(-3, 3),
					life: l, maxLife: l, c: p.trailCol, drag: 2.5, grav: 0.35, bright: 0.4,
				})
			}
		}
		alive = append(alive, *p)
	}
	w.parts = append(alive, w.spawn...)
	w.spawn = w.spawn[:0]
}

func quant(v float32) float64 {
	if v > 4 {
		v = 4
	}
	return math.Floor(float64(v)*20) / 20
}

func to8(v float64) int {
	n := int(v * 255)
	if n > 255 {
		return 255
	}
	if n < 0 {
		return 0
	}
	return n
}

func (w *world) pixel(x, y int) (int, int, int) {
	i := y*w.w + x
	b := w.bg[i]
	if w.solid[i] {
		return to8(b.r), to8(b.g), to8(b.b)
	}
	s := float64(w.starB[i])
	j := i * 3
	lr, lg, lb := quant(w.light[j]), quant(w.light[j+1]), quant(w.light[j+2])
	if m := math.Max(lr, math.Max(lg, lb)); m > 1 {
		e := (m - 1) * 0.2
		lr, lg, lb = lr+e, lg+e, lb+e
	}
	return to8(b.r + s*0.9 + lr), to8(b.g + s*0.9 + lg), to8(b.b + s + lb)
}

type cell struct {
	fg, bg int32
	ch     rune
}

type app struct {
	w          *world
	cols, rows int
	prev       []cell
	buf        []byte
	mode256    bool
}

func to256(r, g, b int) int32 {
	mx, mn := r, r
	for _, v := range []int{g, b} {
		if v > mx {
			mx = v
		}
		if v < mn {
			mn = v
		}
	}
	if mx-mn < 12 {
		if r < 8 {
			return 16
		}
		if r > 238 {
			return 231
		}
		return int32(232 + (r-8)/10)
	}
	q := func(v int) int {
		if v < 48 {
			return 0
		}
		if v < 115 {
			return 1
		}
		return (v - 35) / 40
	}
	return int32(16 + 36*q(r) + 6*q(g) + q(b))
}

func (a *app) enc(r, g, b int) int32 {
	if a.mode256 {
		return to256(r, g, b)
	}
	return int32(r<<16 | g<<8 | b)
}

func (a *app) resize(cols, rows int) {
	a.cols, a.rows = cols, rows
	if a.w == nil {
		a.w = newWorld(cols, rows*2)
	} else {
		a.w.resize(cols, rows*2)
	}
	a.prev = make([]cell, cols*rows)
	a.invalidate()
	a.buf = append(a.buf, "\x1b[0m\x1b[2J"...)
}

func (a *app) invalidate() {
	for i := range a.prev {
		a.prev[i] = cell{fg: -2, bg: -2}
	}
}

func (a *app) color(fg bool, v int32) {
	if fg {
		a.buf = append(a.buf, "\x1b[38;"...)
	} else {
		a.buf = append(a.buf, "\x1b[48;"...)
	}
	if a.mode256 {
		a.buf = append(a.buf, "5;"...)
		a.buf = strconv.AppendInt(a.buf, int64(v), 10)
	} else {
		a.buf = append(a.buf, "2;"...)
		a.buf = strconv.AppendInt(a.buf, int64(v>>16&255), 10)
		a.buf = append(a.buf, ';')
		a.buf = strconv.AppendInt(a.buf, int64(v>>8&255), 10)
		a.buf = append(a.buf, ';')
		a.buf = strconv.AppendInt(a.buf, int64(v&255), 10)
	}
	a.buf = append(a.buf, 'm')
}

func (a *app) render(hintAlpha float64) []byte {
	w := a.w
	curFg, curBg := int32(-1), int32(-1)
	lastR, lastC := -1, -1
	hint := []rune(hintText)
	hintStart := (a.cols - len(hint)) / 2
	showHint := hintAlpha > 0 && hintStart >= 0
	for r := 0; r < a.rows; r++ {
		for c := 0; c < a.cols; c++ {
			tr, tg, tb := w.pixel(c, 2*r)
			br, bg, bb := w.pixel(c, 2*r+1)
			var cl cell
			if hc := c - hintStart; showHint && r == a.rows-1 && hc >= 0 && hc < len(hint) && hint[hc] != ' ' {
				k := hintAlpha * 0.8
				cl = cell{
					fg: a.enc(br+int(float64(220-br)*k), bg+int(float64(220-bg)*k), bb+int(float64(230-bb)*k)),
					bg: a.enc(br, bg, bb), ch: hint[hc],
				}
			} else {
				top, bot := a.enc(tr, tg, tb), a.enc(br, bg, bb)
				if top == bot {
					cl = cell{fg: -1, bg: bot, ch: ' '}
				} else {
					cl = cell{fg: top, bg: bot, ch: '▀'}
				}
			}
			i := r*a.cols + c
			if a.prev[i] == cl {
				continue
			}
			a.prev[i] = cl
			if r != lastR || c != lastC+1 {
				a.buf = append(a.buf, "\x1b["...)
				a.buf = strconv.AppendInt(a.buf, int64(r+1), 10)
				a.buf = append(a.buf, ';')
				a.buf = strconv.AppendInt(a.buf, int64(c+1), 10)
				a.buf = append(a.buf, 'H')
			}
			if cl.ch != ' ' && cl.fg != curFg {
				a.color(true, cl.fg)
				curFg = cl.fg
			}
			if cl.bg != curBg {
				a.color(false, cl.bg)
				curBg = cl.bg
			}
			a.buf = append(a.buf, string(cl.ch)...)
			lastR, lastC = r, c
		}
	}
	out := a.buf
	a.buf = a.buf[:0]
	return out
}

type winsize struct{ row, col, x, y uint16 }

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg)); e != 0 {
		return e
	}
	return nil
}

func termSize(fd int) (int, int, error) {
	var ws winsize
	if err := ioctl(fd, syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); err != nil {
		return 0, 0, err
	}
	return int(ws.col), int(ws.row), nil
}

func makeRaw(fd int) (*syscall.Termios, error) {
	var old syscall.Termios
	if err := ioctl(fd, syscall.TCGETS, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	raw := old
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := ioctl(fd, syscall.TCSETS, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return &old, nil
}

func run() int {
	in, out := int(os.Stdin.Fd()), int(os.Stdout.Fd())
	cols, rows, err := termSize(out)
	if err != nil || cols < 10 || rows < 5 {
		fmt.Fprintln(os.Stderr, "Il faut un vrai terminal (ssh -t).")
		return 1
	}
	old, err := makeRaw(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Il faut un vrai terminal (ssh -t).")
		return 1
	}
	os.Stdout.WriteString("\x1b[?1049h\x1b[?25l\x1b[2J")
	defer func() {
		os.Stdout.WriteString("\x1b[0m\x1b[2J\x1b[?25h\x1b[?1049l")
		ioctl(in, syscall.TCSETS, unsafe.Pointer(old))
	}()

	sig := make(chan os.Signal, 4)
	signal.Notify(sig, syscall.SIGWINCH, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	keys := make(chan byte, 32)
	go func() {
		b := make([]byte, 64)
		for {
			n, err := os.Stdin.Read(b)
			if err != nil {
				close(keys)
				return
			}
			if b[0] == 27 {
				if n == 1 {
					keys <- 'q'
				}
				continue
			}
			for _, c := range b[:n] {
				select {
				case keys <- c:
				default:
				}
			}
		}
	}()

	var deadline <-chan time.Time
	var finaleAt <-chan time.Time
	if d, err := time.ParseDuration(maxDuration); err == nil && d > 0 {
		deadline = time.After(d)
		f := d - 9700*time.Millisecond
		if f < 300*time.Millisecond {
			f = 300 * time.Millisecond
		}
		finaleAt = time.After(f)
	}

	a := &app{}
	a.resize(cols, rows)
	start := time.Now()
	tick := time.NewTicker(time.Second / fps)
	defer tick.Stop()

	for {
		select {
		case s := <-sig:
			if s != syscall.SIGWINCH {
				return 0
			}
			if c, r, err := termSize(out); err == nil && c > 0 && r > 0 {
				a.resize(c, r)
			}
		case k, ok := <-keys:
			if !ok {
				return 0
			}
			switch k {
			case 'q', 'Q', 3, 4:
				return 0
			case ' ':
				kind, c1, c2 := a.w.randomShell()
				a.w.salvo(5, kind, c1, c2)
			case 'f', 'F':
				a.w.finale()
			case 'c', 'C':
				a.mode256 = !a.mode256
				a.invalidate()
			}
		case <-finaleAt:
			a.w.finale()
		case <-deadline:
			return 0
		case <-tick.C:
			a.w.step()
			el := time.Since(start).Seconds()
			alpha := math.Max(0, math.Min(1, (6-el)/1.5))
			if _, err := os.Stdout.Write(a.render(alpha)); err != nil {
				return 0
			}
		}
	}
}

// parseArgs ignores unknown arguments: as a login shell under ForceCommand we get "-c <cmd>".
func parseArgs(args []string) {
	if v, ok := os.LookupEnv("FIREWORKS_MESSAGE"); ok {
		message = v
	}
	for i := 0; i < len(args); i++ {
		a := strings.TrimPrefix(args[i], "-")
		switch {
		case a == "-message" || a == "message" || a == "m":
			if i+1 < len(args) {
				message = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "-message=") || strings.HasPrefix(a, "message="):
			message = a[strings.Index(a, "=")+1:]
		}
	}
}

func main() {
	parseArgs(os.Args[1:])
	code := run()
	if code == 0 {
		fmt.Println("Merci d'etre passe ! A bientot.")
	}
	os.Exit(code)
}
