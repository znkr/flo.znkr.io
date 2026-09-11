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
    // viewport. Giving all of it would leave a screen of blank before the
    // footer; giving half of it brings all but the last heading or two within
    // reach, and the reading line covers the rest.
    static #tailShare = 0.5

    #tocLinks
    #headers
    #list
    #content
    #ticking
    #activeIndex

    constructor() {
        let tocLinks = document.querySelectorAll('.toc a');
        if (tocLinks.length == 0) {
            return
        }
        this.#tocLinks = tocLinks
        this.#list = document.querySelector('.toc section > ul')
        this.#content = document.querySelector('main .content')
        this.#headers = Array.from(this.#tocLinks).map(link => {
            return document.querySelector(`#${link.href.split('#')[1]}`);
        })
        this.#activeIndex = -1;
        this.#pad();
        this.#aim();
        this.#update();

        // The rider is revealed a frame after it is placed, so that the first
        // placement is not animated from the top of the list.
        requestAnimationFrame(() => this.#list.classList.add('tracking'));

        window.addEventListener('scroll', (e) => {
            this.#onScroll()
        })
        window.addEventListener('resize', (e) => {
            this.#pad()
            this.#aim()
            this.#onScroll()
        })
        window.addEventListener('load', (e) => {
            this.#pad()
            this.#aim()
            this.#onScroll()
        })
    }

    // tracking reports whether the layout marks a reading position. Only the
    // sidebar layout draws a rider, and only it is worth scrolling for.
    #tracking() {
        return getComputedStyle(this.#list, '::before').content !== 'none';
    }

    // pad gives the document room to scroll its last headings closer to the top
    // of the viewport. Without it the headings of the last screen cannot be
    // reached, and a link to one of them lands short.
    #pad() {
        if (!this.#tracking()) {
            this.#content.style.setProperty('--tail', '0px');
            return;
        }
        let last = this.#headers[this.#headers.length - 1];
        let top = last.getBoundingClientRect().top + window.scrollY;

        // The room already given is measured rather than remembered, because the
        // layout it is given in is the only one that takes it.
        let given = parseFloat(getComputedStyle(this.#content).paddingBottom) || 0;
        let trailing = document.documentElement.scrollHeight - given - top;
        let room = Math.max(0, window.innerHeight - trailing) * Scroller.#tailShare;
        this.#content.style.setProperty('--tail', `${room}px`);
    }

    // ramp reports where the reading line stops following the top of the
    // viewport: the last heading that can be brought to the top, and the scroll
    // left after it. Past that heading the line runs on to the end of the
    // document, so that the sections below it are reached as the page bottoms
    // out.
    #ramp(starts) {
        let max = document.documentElement.scrollHeight - window.innerHeight;
        let anchor = Math.max(0, starts.findLastIndex(start => start <= max));
        return { max: max, from: starts[anchor], span: max - starts[anchor] };
    }

    // line returns the position in the document that a scroll position reads at.
    #line(y, ramp) {
        let t = ramp.span > 0
            ? Math.min(Math.max((y - ramp.from) / ramp.span, 0), 1)
            : (y >= ramp.max ? 1 : 0);
        return y + window.innerHeight * t;
    }

    // aim points each heading at the scroll position that reads as its own
    // section. For a heading that can be brought to the top of the viewport that
    // is where the browser would stop anyway; for one in the last screen it is
    // not, and the margin makes up the difference.
    #aim() {
        if (!this.#tracking()) {
            this.#headers.forEach(header => header.style.scrollMarginTop = '');
            return;
        }
        let starts = this.#headers.map(h => Math.round(h.getBoundingClientRect().top + window.scrollY));
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
            let y = ramp.span > 0
                ? (target * ramp.span + window.innerHeight * ramp.from) / (ramp.span + window.innerHeight)
                : ramp.max;
            header.style.scrollMarginTop = `${Math.max(0, starts[i] - y)}px`;
        });
    }

    #onScroll() {
        if (!this.#ticking) {
            requestAnimationFrame(this.#update.bind(this));
            this.#ticking = true;
        }
    }

    #update() {
        let starts = this.#headers.map(h => Math.round(h.getBoundingClientRect().top + window.scrollY));
        let line = Math.round(this.#line(window.scrollY, this.#ramp(starts)));

        let i = 0;
        while (i + 1 < starts.length && starts[i + 1] <= line) {
            i++;
        }
        let link = this.#tocLinks[i];
        this.#list.style.setProperty('--rider-top', `${link.offsetTop}px`);
        this.#list.style.setProperty('--rider-height', `${link.offsetHeight}px`);

        if (i !== this.#activeIndex) {
            this.#activeIndex = i;
            this.#tocLinks.forEach(link => link.classList.remove('active'));
            link.classList.add('active');
        }
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