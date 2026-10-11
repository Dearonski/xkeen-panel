const maps = new Map<string, string>()

// Inward normals only, so the filter never samples pixels outside the element
export function displacementMap(w: number, h: number, r: number) {
    const key = `${w}x${h}x${r}`
    const cached = maps.get(key)
    if (cached) return cached

    const bezel = Math.min(18, w / 2, h / 2)
    const canvas = document.createElement('canvas')
    canvas.width = w
    canvas.height = h
    const ctx = canvas.getContext('2d')
    if (!ctx) return ''

    const img = ctx.createImageData(w, h)
    const hx = w / 2
    const hy = h / 2
    const sd = (x: number, y: number) => {
        const qx = Math.abs(x - hx) - hx + r
        const qy = Math.abs(y - hy) - hy + r
        return (
            Math.hypot(Math.max(qx, 0), Math.max(qy, 0)) +
            Math.min(Math.max(qx, qy), 0) -
            r
        )
    }

    for (let y = 0; y < h; y++) {
        for (let x = 0; x < w; x++) {
            const px = x + 0.5
            const py = y + 0.5
            const depth = -sd(px, py)
            let dx = 0
            let dy = 0
            if (depth > 0 && depth < bezel) {
                const nx = sd(px + 0.5, py) - sd(px - 0.5, py)
                const ny = sd(px, py + 0.5) - sd(px, py - 0.5)
                const len = Math.hypot(nx, ny) || 1
                const t = 1 - depth / bezel
                dx = (-nx / len) * t * t
                dy = (-ny / len) * t * t
            }
            const i = (y * w + x) * 4
            img.data[i] = 128 + dx * 127
            img.data[i + 1] = 128 + dy * 127
            img.data[i + 2] = 128
            img.data[i + 3] = 255
        }
    }

    ctx.putImageData(img, 0, 0)
    const url = canvas.toDataURL()
    maps.set(key, url)
    return url
}
