document.addEventListener("DOMContentLoaded", main)

function main() {
    new Scroller()
    for (const table of document.querySelectorAll("table.code-snippet.diff")) {
        new DiffTable(table)
    }
}

class Scroller {
    #tocLinks
    #headers
    #ticking
    #activeHeader

    constructor() {
        let tocLinks = document.querySelectorAll('.toc a');
        if (tocLinks) {
            this.#tocLinks = tocLinks
            this.#headers = Array.from(this.#tocLinks).map(link => {
                return document.querySelector(`#${link.href.split('#')[1]}`);
            })
            this.#update();
            window.addEventListener('scroll', (e) => {
                this.#onScroll()
            })
        }
    }

    #onScroll() {
        if (!this.#ticking) {
            requestAnimationFrame(this.#update.bind(this));
            this.#ticking = true;
        }
    }

    #update() {
        let activeIndex = this.#headers.findIndex((header) => {
            return header.getBoundingClientRect().top > 180;
        });
        if (activeIndex == -1) {
            activeIndex = this.#headers.length - 1;
        } else if (activeIndex > 0) {
            activeIndex--;
        }
        let active = this.#headers[activeIndex];
        if (active !== this.#activeHeader) {
            this.#activeHeader = active;
            this.#tocLinks.forEach(link => link.classList.remove('active'));
            this.#tocLinks[activeIndex].classList.add('active');
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
            this.#addUnfoldCell(group, "unfold", (event) => { this.#unfoldDown(group) }, 2)
        } else if (group.isStart && !group.isEnd) {
            this.#addUnfoldCell(group, "unfold-up", (event) => { this.#unfoldUp(group) }, 2)
        } else if (!group.isStart && group.isEnd) {
            this.#addUnfoldCell(group, "unfold-down", (event) => { this.#unfoldDown(group) }, 2)
        } else {
            this.#addUnfoldCell(group, "unfold-down", (event) => { this.#unfoldDown(group) }, 1)
            this.#addUnfoldCell(group, "unfold-up", (event) => { this.#unfoldUp(group) }, 1)
        }

        let op = group.ctrl.insertCell()
        let code = group.ctrl.insertCell()
        code.classList.add("hunk-desc")
        this.#updateGroupCtrlDesc(group)
    }

    #addUnfoldCell(group, icon, onclick, colSpan) {
        let button = document.createElement("button")
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