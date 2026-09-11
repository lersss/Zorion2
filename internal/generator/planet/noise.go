// internal/generator/planet/noise.go
package planet

import (
	"math"
	"math/rand"
)

// valueNoise — детерминированный вэлью-нойз (3D) с таблицей перестановок.
// Ключевая особенность: шум сэмплируется в 3D, что позволяет текстурировать
// планету как сферу без швов и искажений у края диска.
type valueNoise struct {
	perm [512]int
}

func newValueNoise(seed int64) *valueNoise {
	vn := &valueNoise{}
	p := make([]int, 256)
	for i := range p {
		p[i] = i
	}
	rnd := rand.New(rand.NewSource(seed))
	for i := 255; i > 0; i-- {
		j := rnd.Intn(i + 1)
		p[i], p[j] = p[j], p[i]
	}
	for i := 0; i < 512; i++ {
		vn.perm[i] = p[i&255]
	}
	return vn
}

func (vn *valueNoise) hash(x, y, z int) float64 {
	n := vn.perm[(vn.perm[(vn.perm[x&255]+y)&255]+z)&255]
	return float64(n) / 255.0
}

func (vn *valueNoise) eval(x, y, z float64) float64 {
	xi := int(math.Floor(x))
	yi := int(math.Floor(y))
	zi := int(math.Floor(z))
	xf := x - float64(xi)
	yf := y - float64(yi)
	zf := z - float64(zi)

	u := xf * xf * (3 - 2*xf)
	v := yf * yf * (3 - 2*yf)
	w := zf * zf * (3 - 2*zf)

	c000 := vn.hash(xi, yi, zi)
	c100 := vn.hash(xi+1, yi, zi)
	c010 := vn.hash(xi, yi+1, zi)
	c110 := vn.hash(xi+1, yi+1, zi)
	c001 := vn.hash(xi, yi, zi+1)
	c101 := vn.hash(xi+1, yi, zi+1)
	c011 := vn.hash(xi, yi+1, zi+1)
	c111 := vn.hash(xi+1, yi+1, zi+1)

	nx00 := c000 + (c100-c000)*u
	nx10 := c010 + (c110-c010)*u
	nx01 := c001 + (c101-c001)*u
	nx11 := c011 + (c111-c011)*u

	ny0 := nx00 + (nx10-nx00)*v
	ny1 := nx01 + (nx11-nx01)*v
	return ny0 + (ny1-ny0)*w
}

// fbm — фрактальный шум, возвращает [0,1].
func (vn *valueNoise) fbm(x, y, z float64, octaves int, lacunarity, gain float64) float64 {
	amp := 0.5
	freq := 1.0
	total := 0.0
	norm := 0.0
	for i := 0; i < octaves; i++ {
		total += vn.eval(x*freq, y*freq, z*freq) * amp
		norm += amp
		amp *= gain
		freq *= lacunarity
	}
	return total / norm
}

// ridged — «хребтовый» шум (края/трещины/жгуты). Возвращает [0,1].
func (vn *valueNoise) ridged(x, y, z float64, octaves int, lacunarity, gain float64) float64 {
	amp := 0.5
	freq := 1.0
	total := 0.0
	norm := 0.0
	for i := 0; i < octaves; i++ {
		n := 1.0 - math.Abs(vn.eval(x*freq, y*freq, z*freq)*2-1)
		total += n * n * amp
		norm += amp
		amp *= gain
		freq *= lacunarity
	}
	return total / norm
}

// warp — доменный варп координат (турбулентность, естественные искажения).
func (vn *valueNoise) warp(x, y, z, amount float64) (float64, float64, float64) {
	wx := vn.eval(x+23.17, y-11.9, z+7.41)*amount - amount*0.5
	wy := vn.eval(x-19.3, y+13.07, z-5.53)*amount - amount*0.5
	wz := vn.eval(x+5.19, y+29.3, z-17.71)*amount - amount*0.5
	return x + wx, y + wy, z + wz
}

// spherePoint проецирует пиксель диска на сферу.
// (dx, dy) — смещение от центра в пикселях, radius — радиус диска.
// Возвращает точку на единичной сфере, масштабированную на scale —
// это «мировые» координаты для сэмплинга нойза.
func spherePoint(dx, dy, radius, scale float64) (float64, float64, float64) {
	z := math.Sqrt(math.Max(0, radius*radius-dx*dx-dy*dy)) / radius
	return dx / radius * scale, dy / radius * scale, z * scale
}