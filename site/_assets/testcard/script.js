function main() {
    const gl = document.getElementById("screen").getContext("webgl2")
    if (!gl) {
        return
    }

    prog = createProgram(["frag", "vert"])
    gl.useProgram(prog)

    const posLoc = gl.getAttribLocation(prog, "a_pos")
    const inputScaleLoc = gl.getUniformLocation(prog, "u_inputScale")
    const inputStepLoc = gl.getUniformLocation(prog, "u_inputStep")
    const inputLoc = gl.getUniformLocation(prog, "u_input")
    const dotsLoc = gl.getUniformLocation(prog, "u_dots")
    const randomLoc = gl.getUniformLocation(prog, "u_random")
    const outSizeLoc = gl.getUniformLocation(prog, "u_outSize")
    checkError()

    gl.uniform1i(inputLoc, 0)
    gl.uniform1i(dotsLoc, 1)
    checkError()

    // Setup quad to draw into
    const posBuffer = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, posBuffer);
    gl.enableVertexAttribArray(posLoc);
    gl.vertexAttribPointer(posLoc, 2, gl.FLOAT, false, 0, 0);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([
        1.0, 1.0,
        -1.0, 1.0,
        -1.0, -1.0,
        -1.0, -1.0,
        1.0, -1.0,
        1.0, 1.0]), gl.STATIC_DRAW)
    checkError()

    // Setup testcard texture
    const testcardTexture = gl.createTexture();
    gl.activeTexture(gl.TEXTURE0)
    gl.bindTexture(gl.TEXTURE_2D, testcardTexture)
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, 1, 1, 0, gl.RGBA, gl.UNSIGNED_BYTE, new Uint8Array([0, 0, 0, 255])) // dummy
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
    checkError()
    const testcardImg = new Promise((resolve, reject) => {
        const img = new Image()
        img.onload = () => {
            resolve(img)
        }
        img.src = "/assets/testcard/testcard.svg"
    })
    const testcardCanvas = new OffscreenCanvas(1, 1)

    // Setup phosphor dots texture
    const dotsTexture = gl.createTexture()
    gl.activeTexture(gl.TEXTURE1)
    gl.bindTexture(gl.TEXTURE_2D, dotsTexture)
    const dotsArray = new Uint8Array(6*6*4)
    for (let y = 0; y < 6; y++) {
        for (let x = 0; x < 6; x++) {
            let r = 0
            let g = 0
            let b = 0
            v = 255
            if (y == 0 &&  x < 3 || y == 3 && x >= 3) {
               v = 0 // darken some rows to create the impression of separate plasma dots
            }
            if (x % 3 == 0) {
                r = v
            } else if (x % 3 == 1) {
                g = v
            } else if (x % 3 == 2) {
                b = v
            }
            dotsArray[y*6*4 + x*4 + 0] = r
            dotsArray[y*6*4 + x*4 + 1] = g
            dotsArray[y*6*4 + x*4 + 2] = b
            dotsArray[y*6*4 + x*4 + 3] = 255
        }
    }
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, 6, 6, 0, gl.RGBA, gl.UNSIGNED_BYTE, dotsArray)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.REPEAT)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.REPEAT)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
    checkError()

    requestAnimationFrame(draw)

    function draw() {
        const width = gl.canvas.clientWidth
        const height = gl.canvas.clientHeight
        if (gl.canvas.width != width || gl.canvas.height != height) {
            gl.canvas.width = width
            gl.canvas.height = height
            gl.viewport(0, 0, width, height)
            gl.uniform2f(outSizeLoc, width, height)
            renderTestcard(width, height)
        }

        gl.uniform2f(randomLoc, Math.random(), Math.random())

        const s = Math.min(testcardCanvas.width/width, testcardCanvas.height/height)
        gl.uniform2f(inputScaleLoc, s*width/testcardCanvas.width, s*height/testcardCanvas.height)

        gl.clearColor(0.0, 0.0, 0.0, 0.0);
        gl.clear(gl.COLOR_BUFFER_BIT);
        gl.drawArrays(gl.TRIANGLES, 0, 6)

        requestAnimationFrame(draw)
    }

    function renderTestcard(width, height) {
        testcardImg.then((img) => {
            const s = Math.max(width/img.width, height/img.height)
            const w = Math.max(width, s * img.width)
            const h = Math.max(height, s * img.height)
            if (testcardCanvas.width == w && testcardCanvas.height == h) {
                return
            }
            testcardCanvas.width = w
            testcardCanvas.height = h
            const ctx = testcardCanvas.getContext("2d")
            ctx.drawImage(img, 0, 0, w, h)
            gl.activeTexture(gl.TEXTURE0)
            gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, gl.RGBA, gl.UNSIGNED_BYTE, testcardCanvas)
            gl.uniform2f(inputStepLoc, 1/w, 1/h)
        })
    }

    function createProgram(ids) {
        const p = gl.createProgram();

        for (const id of ids) {
            const script = document.getElementById(id)
            const src = script.text
            var type = undefined
            if (script.type == "x-shader/x-vertex") {
                type = gl.VERTEX_SHADER
            } else if (script.type == "x-shader/x-fragment") {
                type = gl.FRAGMENT_SHADER
            }

            const s = gl.createShader(type)
            gl.shaderSource(s, src)
            gl.compileShader(s)
            if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) {
                const info = gl.getShaderInfoLog(s);
                throw `Could not compile WebGL shader ${id}.\n\n${info}`;
            }

            gl.attachShader(p, s)
        }

        gl.linkProgram(p)
        if (!gl.getProgramParameter(p, gl.LINK_STATUS)) {
            const info = gl.getProgramInfoLog(p);
            throw `Could not compile WebGL program.\n\n${info}`
        }
        return p
    }

    function checkError() {
        const error = gl.getError()
        if (error == gl.NO_ERROR) {
            return
        }

        var msg = undefined
        switch (error) {
            case gl.INVALID_ENUM:
                msg = "INVALID_ENUM"
                break
            case gl.INVALID_VALUE:
                msg = "INVALID_VALUE"
                break
            case gl.INVALID_OPERATION:
                msg = "INVALID_VALUE"
                break
            case gl.INVALID_FRAMEBUFFER_OPERATION:
                msg = "INVALID_FRAMEBUFFER_OPERATION"
                break
            case gl.OUT_OF_MEMORY:
                msg = "OUT_OF_MEMORY"
                break
            case gl.CONTEXT_LOST_WEBGL:
                msg = "CONTEXT_LOST_WEBGL"
                break
        }
        throw `WebGL error: ${msg}`
    }
}

document.addEventListener("DOMContentLoaded", main)