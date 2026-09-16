// Reloads the page whenever the dev server rebuilds the site, and shows what
// the build warned about or why it failed. Only served by "gen serve", never
// packed into the generated site.
//
// Wrapped in a function because this shares the global scope with the site's
// own script.js.
(() => {
    const key = "reload-scroll:" + location.pathname

    const y = loadScroll()
    if (y !== null) {
        addEventListener("load", () => window.scrollTo(0, +y))
    }

    // The server reports a status holding a version that changes on every
    // rebuild. The first one seen sets the baseline; a different one means the
    // site changed under us. The version is a timestamp, so restarting the
    // server reloads the page too.
    //
    // A failed rebuild leaves the version alone and sets the error instead:
    // the page shown is still current, it is the save that was rejected.
    let version = null
    const events = new EventSource("/_dev/reload")
    events.onmessage = (e) => {
        const status = JSON.parse(e.data)
        if (version === null) {
            version = status.version
        } else if (status.version !== version) {
            storeScroll(window.scrollY)
            location.reload()
            return
        }
        show("error", status.error)
        show("warnings", status.warnings)
    }

    const panels = {}
    const styles = {
        error: "background:#2a1414;color:#f8e0e0",
        warnings: "background:#2a2414;color:#f8f0d0",
    }

    // show puts text in the panel named by kind, removing the panel when text
    // is empty. The panels are stacked at the bottom of the viewport, error
    // below warnings.
    function show(kind, text) {
        let panel = panels[kind]
        if (!text) {
            if (panel) {
                panel.remove()
                delete panels[kind]
                place()
            }
            return
        }
        if (!panel) {
            panel = document.createElement("pre")
            panel.style.cssText = "position:fixed;left:0;right:0;margin:0;padding:1rem;" +
                "max-height:40vh;overflow:auto;box-sizing:border-box;z-index:2147483647;" +
                "font:14px/1.4 monospace;white-space:pre-wrap;" + styles[kind]
            panels[kind] = panel
            document.body.append(panel)
        }
        panel.textContent = text
        place()
    }

    function place() {
        let bottom = 0
        for (const kind of ["error", "warnings"]) {
            const panel = panels[kind]
            if (!panel) {
                continue
            }
            panel.style.bottom = bottom + "px"
            bottom += panel.offsetHeight
        }
    }

    // sessionStorage throws if the browser blocks site data. Reloading without
    // the scroll position is still better than not reloading.

    function loadScroll() {
        try {
            const y = sessionStorage.getItem(key)
            sessionStorage.removeItem(key)
            return y
        } catch {
            return null
        }
    }

    function storeScroll(y) {
        try {
            sessionStorage.setItem(key, y)
        } catch {}
    }
})()
