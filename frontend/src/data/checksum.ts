import {useEffect, useRef, useState} from 'preact/hooks';
import {PlaysetChecksum} from '../../wailsjs/go/main/App';
import type {main} from '../../wailsjs/go/models';

// The multiplayer checksum: the four characters a Paradox game shows on its main menu, which
// every player in a multiplayer game has to share. The backend (internal/checksum, checksum.go)
// works it out from the saved playset without starting the game; this is the interface half -
// when to ask, and how to say what came back.

export interface ChecksumState {
    // 'none': nothing to show (asked for nothing yet, or a game with no known scheme).
    kind: 'none' | 'calculating' | 'ready' | 'unavailable';
    // The saved playset it was asked for.
    forName: string;
    // The code, files and mods that went into it - while calculating, those of the previous
    // answer, so the number does not flicker away and back for a calculation that takes a moment.
    value: string;
    files: number;
    mods: number;
    // Why there is no code, in words for the user.
    reason: string;
    warnings: string[];
}

export const NO_CHECKSUM: ChecksumState = {kind: 'none', forName: '', value: '', files: 0, mods: 0, reason: '', warnings: []};

function fromResult(name: string, r: main.PlaysetChecksumResult): ChecksumState {
    if (r.Status === 'ready') {
        return {kind: 'ready', forName: name, value: r.Checksum, files: r.Files, mods: r.Mods, reason: '', warnings: r.Warnings ?? []};
    }
    if (r.Status === 'unavailable') {
        return {...NO_CHECKSUM, kind: 'unavailable', forName: name, reason: r.Reason};
    }
    return NO_CHECKSUM;
}

export interface PlaysetChecksumControl {
    state: ChecksumState;
    // Works out the checksum of the saved playset called name (after it was saved or loaded).
    calculate(name: string): void;
    // Works it out again for the last playset asked about, e.g. after mods changed on disk.
    refresh(): void;
}

// usePlaysetChecksum keeps the checksum of gameId's playset. A newer request replaces an older
// one still running (only the latest answer is used), and switching game clears it.
export function usePlaysetChecksum(gameId: string): PlaysetChecksumControl {
    const [state, setState] = useState<ChecksumState>(NO_CHECKSUM);
    const latest = useRef(0);
    const lastName = useRef('');
    const gameRef = useRef(gameId);
    gameRef.current = gameId;

    useEffect(() => {
        latest.current++;
        lastName.current = '';
        setState(NO_CHECKSUM);
    }, [gameId]);

    const control = useRef<PlaysetChecksumControl | null>(null);
    if (control.current === null) {
        control.current = {
            state,
            calculate(name: string) {
                const trimmed = name.trim();
                if (!trimmed || !gameRef.current) return;
                const mine = ++latest.current;
                lastName.current = trimmed;
                setState((prev) => (prev.kind === 'ready' && prev.forName === trimmed
                    ? prev
                    : {...NO_CHECKSUM, kind: 'calculating', forName: trimmed}));
                PlaysetChecksum(gameRef.current, trimmed).then((r) => {
                    if (mine === latest.current) setState(fromResult(trimmed, r));
                }).catch((err) => {
                    if (mine === latest.current) setState({...NO_CHECKSUM, kind: 'unavailable', forName: trimmed, reason: String(err)});
                });
            },
            refresh() {
                if (lastName.current) control.current!.calculate(lastName.current);
            },
        };
    }
    control.current.state = state;
    return control.current;
}

// Whether the checksum is worth showing for the playset on screen: only for the playset it was
// worked out for (a renamed, deleted or new one has none yet).
export function checksumShown(state: ChecksumState, playsetName: string): boolean {
    return state.kind !== 'none' && state.forName === playsetName.trim();
}

// What the small checksum readouts (above Play, in the launch dialog) show.
export interface ChecksumBadge {
    // The code, or a word standing in for it.
    text: string;
    // 'ok' a code to share, 'busy' still working, 'warn' none to give (or one about to change).
    tone: 'ok' | 'busy' | 'warn';
}

// checksumBadge is null when there is nothing to show for the playset on screen.
export function checksumBadge(state: ChecksumState, playsetName: string, unsaved: boolean): ChecksumBadge | null {
    if (!checksumShown(state, playsetName)) return null;
    if (state.kind === 'calculating') return {text: 'calculating...', tone: 'busy'};
    if (state.kind === 'unavailable') return {text: 'unavailable', tone: 'warn'};
    return {text: state.value, tone: unsaved ? 'warn' : 'ok'};
}
