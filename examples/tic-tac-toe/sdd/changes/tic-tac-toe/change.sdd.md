```change
name: tic-tac-toe
status: planning
packages: []
```

## Why

The user wants a tic-tac-toe game they can open in a browser. One static HTML page is enough: no server, no build step. A native app or online play would be more work for no stated benefit.

## User request

We need a tic-tac-toe game in HTML that two people can play on one screen.

## Acceptance

- Opening `index.html` in a browser shows an empty 3×3 board.
- Two players take turns placing X and O, and the page announces a win or a draw.

## Requirements

- Q1 The page SHALL let two players alternate X and O on an empty 3×3 board.
- Q2 The page SHALL end the game on three in a row or a full board, and say which.

## Scenarios

- S1 (Q1) WHEN a player clicks a square that is already taken THEN the mark does not change and the turn does not pass.
- S2 (Q2) WHEN X completes a row, column or diagonal THEN the page shows "X wins" and further clicks do nothing.
- S3 (Q2) WHEN the ninth mark fills the board with no line THEN the page shows "Draw".

## Decisions

- D1 One self-contained `index.html` with inline script. Rejected: a framework or bundler because the request needs no build step.

## Risks

- A click handler left active after the game ends lets play continue → the end state disables the board, covered by S2.

Deploy: n/a (a static file opened locally).

Rollback: n/a (nothing persists).

## Observed

- `ls` in the project root printed only `README.md`; there is no existing page to extend.

## Assumptions

- Both players share one mouse; there is no computer opponent.

## Non-goals

- Online play, scores across games, or a computer opponent.

## Deferred

(Optional — other slugs that own split acceptance.)

## Tasks

- [ ] 1.1 Add `index.html` with a 3×3 board of buttons and a status line. verify: opening the page shows nine empty squares. covers S1
- [ ] 1.2 Alternate X and O on click and ignore taken squares. verify: clicking a taken square keeps its mark. covers S1
- [ ] 1.3 Detect a win or a full board, show the result and disable the board. verify: a won game shows "X wins" and ignores clicks. covers S2,S3

## Validate

- [ ] V1.1 manual: open `index.html`, play X to a diagonal win, and see "X wins" covers S2
- [ ] V1.2 manual: play to a full board with no line and see "Draw" covers S1,S3

## Review

- None: a second reader checked each scenario against the tasks and found every scenario covered and no conflicting decision.
