document.addEventListener("DOMContentLoaded", main)

function main() {
    new Scroller()
    if (window.matchMedia("(hover: hover)").matches) {
        for (const mark of document.querySelectorAll(".footnote-mark")) {
            let note = mark.popoverTargetElement
            if (note != null) {
                new Footnote(mark, note)
            }
        }
    }
    for (const table of document.querySelectorAll("table.code-snippet.diff")) {
        new DiffTable(table)
    }
}

// Footnote opens a note while the pointer rests on its mark and closes it once
// the pointer has left. The mark is a button with a popovertarget, so tap,
// keyboard, Escape and dismiss are the browser's; a click made with the pointer
// is not, because on a device that hovers the note is open already.
class Footnote {
    static #openDelay = 120
    static #closeDelay = 250

    #note
    #timer
    #byHover

    constructor(mark, note) {
        this.#note = note
        this.#watch(mark)
        this.#watch(note)
        mark.addEventListener("click", (e) => this.#clicked(e))
    }

    // watch opens the note while the pointer is over el. The delays are what
    // let the pointer cross the gap between a mark and its note without the
    // note closing on the way.
    #watch(el) {
        el.addEventListener("pointerenter", () => this.#schedule(true, Footnote.#openDelay))
        el.addEventListener("pointerleave", () => this.#schedule(false, Footnote.#closeDelay))
    }

    #schedule(show, delay) {
        clearTimeout(this.#timer)
        if (!show && !this.#byHover) {
            return
        }
        this.#timer = setTimeout(() => this.#toggle(show), delay)
    }

    // toggle opens or closes the note and records whether hover opened it. A
    // note that is open already was opened from the keyboard, and closing that
    // one is the reader's to do.
    #toggle(show) {
        if (show && this.#note.matches(":popover-open")) {
            return
        }
        this.#byHover = show
        this.#note.togglePopover(show)
    }

    // clicked drops a click made with the pointer: the note is open already,
    // and the popover it would toggle is the one hover is holding. A keyboard
    // activation carries no pointer, and opens the note as it always did.
    #clicked(e) {
        if (e.detail > 0) {
            e.preventDefault()
        }
    }
}

class Scroller {
    // The share of the room the last heading would need to reach the top of the
    // reading area. Giving all of it would leave a screen of blank before the
    // footer; giving half of it brings all but the last heading or two within
    // reach, and the reading line covers the rest.
    static #tailShare = 0.5

    #tocLinks
    #headers
    #content
    #ticking
    #activeIndex
    #aside
    #nav
    #bar
    #path
    #label
    #button
    #inset
    #place

    constructor() {
        // A browser that cannot parse :popover-open has no sheet to open, and
        // the stylesheet leaves out the table of contents there.
        let tocLinks = document.querySelectorAll('.toc nav a');
        if (tocLinks.length == 0 || !CSS.supports('selector(:popover-open)')) {
            return
        }
        this.#tocLinks = tocLinks
        this.#content = document.querySelector('main .content')
        this.#headers = Array.from(this.#tocLinks).map(link => {
            return document.querySelector(`#${link.href.split('#')[1]}`);
        })
        this.#activeIndex = -1;
        // The label the page is served with.
        this.#label = 'contents';
        this.#aside = document.querySelector('.toc')
        this.#nav = this.#aside.querySelector('nav')
        this.#bar = this.#aside.querySelector('.toc-bar')
        this.#path = this.#aside.querySelector('.toc-path')
        this.#button = this.#aside.querySelector('.toc-current')
        this.#button.addEventListener('click', (e) => this.#toggle(e))
        this.#nav.addEventListener('click', (e) => {
            if (e.target.closest('a')) {
                this.#close()
            }
        })
        this.#relayout();

        window.addEventListener('scroll', (e) => {
            this.#onScroll()
        })
        window.addEventListener('resize', (e) => {
            this.#relayout()
        })
        window.addEventListener('load', (e) => {
            this.#relayout()
        })

        // A change in the width of the text reflows it, and the reader is
        // returned to the place they were reading.
        new ResizeObserver(() => this.#reflowed()).observe(this.#content);
    }

    // relayout adjusts to a new viewport size or a change in the page's layout.
    #relayout() {
        // The bar pads the document's scroll area by its height.
        this.#inset = parseFloat(getComputedStyle(document.documentElement).scrollPaddingTop) || 0;
        this.#pad();
        this.#aim();
        this.#onScroll();
    }

    // reflowed closes the sheet and returns to the place the reading line was at
    // before the width of the text changed: the same share of the same stretch
    // of content, or the top of the page for a line above the content.
    #reflowed() {
        let width = this.#content.offsetWidth;
        if (!this.#place || this.#place.width === width) {
            return;
        }
        this.#close();
        this.#relayout();
        let k = this.#place.index;
        if (k < 0) {
            window.scrollTo({ top: 0, behavior: 'instant' });
        } else {
            let starts = this.#starts();
            let bounds = this.#bounds(starts);
            let target = bounds[k] + this.#place.share * (bounds[k + 1] - bounds[k]);
            window.scrollTo({ top: this.#scrollFor(target, this.#ramp(starts)), behavior: 'instant' });
        }
        this.#place.width = width;
        this.#onScroll();
    }

    // locate returns the place of a reading line: the index of the stretch of
    // content it is in and the share of that stretch above it. The stretches are
    // the ones bounds returns; a line above the content has index -1.
    #locate(line, starts) {
        let bounds = this.#bounds(starts);
        let k = Math.min(bounds.findLastIndex(b => b <= line), bounds.length - 2);
        if (k < 0) {
            return { index: -1, share: 0 };
        }
        return { index: k, share: Scroller.#clamp((line - bounds[k]) / (bounds[k + 1] - bounds[k])) };
    }

    // bounds returns the edges of the stretches of content, measured as starts
    // measures headings: its top, each heading and the end of the document.
    #bounds(starts) {
        return [this.#readTop(this.#content), ...starts, document.documentElement.scrollHeight - this.#inset];
    }

    // starts returns, for each heading, the scroll position that brings it to
    // the top of the reading area.
    #starts() {
        return this.#headers.map(header => this.#readTop(header));
    }

    // readTop returns the scroll position that brings el to the top of the
    // reading area.
    #readTop(el) {
        return Scroller.#pageTop(el) - this.#inset;
    }

    static #pageTop(el) {
        return Math.round(el.getBoundingClientRect().top + window.scrollY);
    }

    static #clamp(t) {
        return Math.min(Math.max(t, 0), 1);
    }

    // pad gives the document room to scroll its last headings closer to the top
    // of the reading area. Without it the headings of the last screen cannot be
    // reached, and a link to one of them lands short.
    #pad() {
        let last = this.#headers[this.#headers.length - 1];
        let top = Scroller.#pageTop(last);

        // The room already given is measured rather than remembered, because a
        // resize changes how much of it is needed.
        let given = parseFloat(getComputedStyle(this.#content).paddingBottom) || 0;
        let trailing = document.documentElement.scrollHeight - given - top;
        let room = Math.max(0, window.innerHeight - this.#inset - trailing) * Scroller.#tailShare;
        this.#content.style.setProperty('--tail', `${room}px`);
    }

    // ramp reports where the reading line stops following the top of the
    // reading area: the scroll position that brings the last reachable heading
    // there, and the scroll left after it. Past that heading the line runs on to
    // the end of the document, so that the sections below it are reached as the
    // page bottoms out.
    #ramp(starts) {
        let max = document.documentElement.scrollHeight - window.innerHeight;
        let anchor = Math.max(0, starts.findLastIndex(start => start <= max));
        let from = starts[anchor];
        return { max: max, from: from, span: max - from };
    }

    // line returns the point a scroll position reads at, measured as starts
    // measures headings.
    #line(y, ramp) {
        let t = ramp.span > 0
            ? Scroller.#clamp((y - ramp.from) / ramp.span)
            : (y >= ramp.max ? 1 : 0);
        return y + (window.innerHeight - this.#inset) * t;
    }

    // scrollFor returns the scroll position whose reading line is at target.
    #scrollFor(target, ramp) {
        if (target <= ramp.from) {
            return target;
        }
        if (ramp.span <= 0) {
            return ramp.max;
        }
        let height = window.innerHeight - this.#inset;
        return (target * ramp.span + height * ramp.from) / (ramp.span + height);
    }

    // aim points each heading at the scroll position that reads as its own
    // section. For a heading that can be brought to the top of the reading area
    // that is where the browser would stop anyway; for one in the last screen it
    // is not, and the margin makes up the difference.
    #aim() {
        let starts = this.#starts();
        let ramp = this.#ramp(starts);

        this.#headers.forEach((header, i) => {
            if (starts[i] <= ramp.from) {
                header.style.scrollMarginTop = '';
                return;
            }
            // A line landing exactly on a heading is a rounding error away from
            // reading as the section before it, so aim just inside the section.
            let target = starts[i] + 2;
            if (i + 1 < starts.length) {
                target = Math.min(target, starts[i + 1] - 1);
            }
            let y = this.#scrollFor(target, ramp);
            header.style.scrollMarginTop = `${Math.max(0, starts[i] - y)}px`;
        });
    }

    // toggle expands or collapses the list in place below the header. Pinned,
    // the button opens the list as a sheet instead, which is the popover's own
    // doing.
    #toggle(e) {
        if (!this.#expanded && this.#bar.classList.contains('pinned')) {
            return;
        }
        e.preventDefault();
        if (this.#expanded) {
            this.#collapse();
        } else {
            this.#expand();
        }
    }

    // expanded reports whether the list is expanded in place.
    get #expanded() {
        return this.#button.getAttribute('aria-expanded') === 'true';
    }

    #expand() {
        this.#button.setAttribute('aria-expanded', 'true');
        this.#onScroll();
    }

    // collapse closes the list expanded in place. A list scrolled out above the
    // viewport takes its height with it, and the page scrolls to keep the text
    // in place.
    #collapse() {
        let above = this.#aside.getBoundingClientRect().bottom <= 0;
        let top = this.#content.getBoundingClientRect().top;
        this.#button.removeAttribute('aria-expanded');
        // Browsers with scroll anchoring have kept the text in place already;
        // the others are left with the whole height to make up.
        let shift = this.#content.getBoundingClientRect().top - top;
        if (above && shift !== 0) {
            window.scrollBy({ top: shift, behavior: 'instant' });
        }
        this.#onScroll();
    }

    // close closes the list, open as a sheet or expanded in place.
    #close() {
        if (this.#nav.matches(':popover-open')) {
            this.#nav.hidePopover();
        }
        if (this.#expanded) {
            this.#collapse();
        }
    }

    // crumbs returns the text of link and of each entry it is nested in,
    // outermost first.
    #crumbs(link) {
        let crumbs = [];
        for (let li = link.closest('li'); li; li = li.parentElement.closest('li')) {
            crumbs.unshift(li.querySelector(':scope > a').textContent);
        }
        return crumbs;
    }

    #onScroll() {
        if (!this.#ticking) {
            requestAnimationFrame(this.#update.bind(this));
            this.#ticking = true;
        }
    }

    #update() {
        // A list expanded in place collapses once it has scrolled out of view,
        // and an open sheet whose bar has come loose from the top becomes the
        // list expanded in place.
        if (this.#expanded && this.#aside.getBoundingClientRect().bottom <= 0) {
            this.#collapse();
        } else if (this.#nav.matches(':popover-open') && this.#bar.getBoundingClientRect().top >= 1) {
            this.#nav.hidePopover();
            this.#expand();
        }

        // Every measurement comes before the first write, so that the frame lays
        // out once.
        let starts = this.#starts();
        let ramp = this.#ramp(starts);
        let place = this.#locate(Math.round(this.#line(window.scrollY, ramp)), starts);
        let width = this.#content.offsetWidth;
        let bar = this.#bar.getBoundingClientRect();

        // The stretch above the first heading reads as the first section.
        let i = Math.max(0, place.index - 1);
        let link = this.#tocLinks[i];

        // A width the text has not reflowed to yet is left for reflowed, which
        // needs the place the line was at before it.
        if (!this.#place || this.#place.width === width) {
            this.#place = { ...place, width: width };
        }

        if (i !== this.#activeIndex) {
            this.#activeIndex = i;
            this.#tocLinks.forEach(link => link.classList.remove('active'));
            link.classList.add('active');
        }

        // The sticky bar is pinned once its top has reached the viewport's.
        // Expanded in place, it does not stick.
        let pinned = !this.#expanded && bar.top < 1;

        // In place below the header the bar names the list. Pinned, it names
        // the section being read, and above the first heading the article
        // title stands alone. The label is keyed by what it shows.
        let label = !pinned ? 'contents' : (place.index > 0 ? i : 'title');
        if (label !== this.#label) {
            this.#label = label;
            let crumbs = !pinned ? ['Contents'] : (place.index > 0 ? this.#crumbs(link) : []);
            this.#path.replaceChildren(...crumbs.map(text => {
                let crumb = document.createElement('span');
                crumb.textContent = text;
                return crumb;
            }));
        }

        this.#bar.classList.toggle('pinned', pinned);
        this.#bar.style.setProperty('--progress', ramp.max > 0 ? Scroller.#clamp(window.scrollY / ramp.max) : 1);
        this.#ticking = false;
    }
}

class DiffTable {
    static #maxContext = 3
    static #maxUnfold = 20

    #table

    constructor(table) {
        this.#table = table

        var prev = 0
        for (let i = 0; i < table.rows.length; i++) {
            let row = table.rows[i]
            let code = row.querySelector(".code code")
            if (code == null) {
                continue
            }
            let ident = DiffTable.#scoreIdent(code.innerText)
            if (ident == Number.MAX_SAFE_INTEGER && prev == 0) {
                table.rows[i-1].setAttribute("data-block-end", "")
            }
            else if (ident > prev && prev == 0) {
                table.rows[i-1].setAttribute("data-block-start", "")
            }
            prev = ident
        }

        for (let group of DiffTable.#groupMatches(table)) {
            let maxContextTotal = 0
            if (!group.isStart) {
                maxContextTotal += DiffTable.#maxContext
            }
            if (!group.isEnd) {
                maxContextTotal += DiffTable.#maxContext
            }
            if (group.last.rowIndex - group.first.rowIndex < maxContextTotal + 1) {
                // Don't hide if the number of hidden rows is smaller than context rows plus 1 row
                // for the control surface.
                this.#dropGroup(group)
                continue
            }

            // Don't hide the context if there is a previous or next edit. There's generally no previous
            // or next edit if both diffed file starts or ends with the same rows. In those cases, we
            // don't want context and instead hide all matches.
            if (!group.isStart) {
                for (let i = 0; i < DiffTable.#maxContext; i++) {
                    group.first = group.first.nextSibling
                }
            }
            if (!group.isEnd) {
                for (let i = 0; i < DiffTable.#maxContext; i++) {
                    group.last = group.last.previousSibling
                }
            }
            this.#hideGroup(group)
            this.#updateGroupCtrl(group)
            this.#notify(group)
        }
    }

    static #groupMatches(table) {
        let groups = []
        let first = null
        let prev = null
        let isStart = true
        for (let i = 0; i < table.rows.length; i++) {
            let row = table.rows[i]
            switch (row.dataset.op) {
                case "match":
                    if (first == null) {
                        first = row
                    }
                    break
                case "delete":
                case "insert":
                    if (first != null) {
                        // i must be > 0 because we always start with first == null
                        let group = {
                            first: first,
                            last: table.rows[i - 1],
                            prev: prev,
                            next: null,
                            ctrl: null,
                            isStart: isStart,
                            isEnd: false,
                        }
                        if (prev != null) {
                            prev.next = group
                        }
                        first = null
                        prev = group
                        groups.push(group)
                    }
                    isStart = false
                    break
                default:
                    // ignore non-op rows
                    break
            }
        }
        if (first != null) {
            // i must be > 0 because we always start with first == null
            let group = {
                first: first,
                last: table.rows[table.rows.length - 1],
                prev: prev,
                next: null,
                ctrl: null,
                isStart: isStart,
                isEnd: true,
            }
            if (prev != null) {
                prev.next = group
            }
            groups.push(group)
        }
        return groups
    }

    #hideGroup(group) {
        let c = group.first
        while (true) {
            c.style = "display: none;"
            if (c == group.last) {
                break
            }
            c = c.nextSibling
        }
    }

    #dropGroup(group) {
        if (group.prev != null) {
            group.prev.next = group.next
        }
        if (group.next != null) {
            group.next.prev = group.prev
        }
        if (group.ctrl != null) {
            this.#table.deleteRow(group.ctrl.rowIndex)
        }
    }

    #updateGroupCtrl(group) {
        if (group.ctrl != null) {
            this.#table.deleteRow(group.ctrl.rowIndex)
        }
        group.ctrl = this.#table.insertRow(group.first.rowIndex)
        group.ctrl.classList.add("ctrl")

        if (!group.isEnd && !group.isStart && group.last.rowIndex - group.first.rowIndex <= DiffTable.#maxUnfold) {
            this.#addUnfoldCell(group, "unfold", "Unfold", (event) => { this.#unfoldDown(group) }, 2)
        } else if (group.isStart && !group.isEnd) {
            this.#addUnfoldCell(group, "unfold-up", "Unfold Up", (event) => { this.#unfoldUp(group) }, 2)
        } else if (!group.isStart && group.isEnd) {
            this.#addUnfoldCell(group, "unfold-down", "Unfold Down", (event) => { this.#unfoldDown(group) }, 2)
        } else {
            this.#addUnfoldCell(group, "unfold-down", "Unfold Down", (event) => { this.#unfoldDown(group) }, 1)
            this.#addUnfoldCell(group, "unfold-up", "Unfold Up", (event) => { this.#unfoldUp(group) }, 1)
        }

        let op = group.ctrl.insertCell()
        let code = group.ctrl.insertCell()
        code.classList.add("hunk-desc")
        this.#updateGroupCtrlDesc(group)
    }

    #addUnfoldCell(group, icon, title, onclick, colSpan) {
        let button = document.createElement("button")
        button.setAttribute("title", title)
        button.classList.add("fold-button")
        button.classList.add(icon)
        //button.innerHTML = "<svg viewBox=\"0 0 16 16\"><use href=\"#"+icon+"\" /></svg>"

        button.onclick = onclick
        let cell = group.ctrl.insertCell()
        cell.classList.add("fold-ctrl")
        cell.colSpan = colSpan
        cell.appendChild(button)
    }

    #unfoldDown(group) {
        let c = group.first
        for (let i = 0; i < DiffTable.#maxUnfold; i++) {
            c.style = ""
            if (c == group.last) {
                // everything's unhidden, we're done.
                break
            }
            c = c.nextSibling
        }
        if (c == group.last) {
            c.style = ""
            this.#dropGroup(group)
        } else {
            group.first = c
            this.#updateGroupCtrl(group)
        }
        this.#notify(group)
    }

    #unfoldUp(group) {
        let c = group.last
        for (let i = 0; i < DiffTable.#maxUnfold; i++) {
            c.style = ""
            if (c == group.first) {
                // everything's unhidden, we're done.
                break
            }
            c = c.previousSibling
        }
        if (c == group.first) {
            c.style = ""
            this.#dropGroup(group)
        } else {
            group.last = c
            this.#updateGroupCtrl(group)
        }
        this.#notify(group)
    }

    #updateGroupCtrlDesc(group) {
        let yLineno = -1
        let xLineno = -1
        let xLines = 0
        let yLines = 0

        let end = null
        if (group.next != null) {
            end = group.next.first
        }
        for (let c = group.last.nextSibling; c != end; c = c.nextSibling) {
            if (c.dataset.xLineno > 0) {
                if (xLineno < 0) {
                    xLineno = c.dataset.xLineno
                }
                xLines++
            }
            if (c.dataset.yLineno > 0) {
                if (yLineno < 0) {
                    yLineno = c.dataset.yLineno
                }
                yLines++
            }
        }

        var leader = ""
        for (let c = group.last; c != null; c = c.previousSibling) {
            if (c.dataset.blockStart != null) {
                leader = c.querySelector(".code code").innerText
                break
            } else if (c.dataset.blockEnd != null) {
                break
            }
        }
        if (xLineno > 0 && yLineno > 0) {
            let desc = group.ctrl.getElementsByClassName("hunk-desc")[0]
            desc.textContent = `@@ -${xLineno},${xLines} +${yLineno},${yLines} @@ ${leader}`
        }
    }

    #notify(group) {
        if (group.prev != null) {
            this.#updateGroupCtrlDesc(group.prev)
        }
    }

    static #scoreIdent(s) {
        let score = 0
        for (const c of s) {
            switch (c) {
                case ' ':
                    score++
                    break
                case '\t':
                    score += 4
                    break
                case '\n':
                case '\r':
                    break
                default:
                    return score
            }
        }
        return Number.MAX_SAFE_INTEGER
    }
}