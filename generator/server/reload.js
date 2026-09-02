// Reloads the page whenever the dev server rebuilds the site. Only served by
// "gen serve", never packed into the generated site.
//
// Wrapped in a function because this shares the global scope with the site's
// own script.js.
(() => {
    const key = "reload-scroll:" + location.pathname

    const y = loadScroll()
    if (y !== null) {
        addEventListener("load", () => window.scrollTo(0, +y))
    }

    // The server reports a version that changes on every rebuild. The first
    // one seen sets the baseline; a different one means the site changed under
    // us. The version is a timestamp, so restarting the server reloads the
    // page too.
    let version = null
    const events = new EventSource("/_dev/reload")
    events.onmessage = (e) => {
        if (version === null) {
            version = e.data
            return
        }
        if (e.data !== version) {
            storeScroll(window.scrollY)
            location.reload()
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
