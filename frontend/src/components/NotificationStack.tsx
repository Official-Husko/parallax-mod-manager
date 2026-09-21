import './NotificationStack.css';
import {h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';
import {dismiss, type Notification, useNotifications} from '../data/notifications';

const ICONS: Record<string, string> = {
    info: 'fa-circle-info',
    success: 'fa-circle-check',
    warning: 'fa-circle-exclamation',
    error: 'fa-triangle-exclamation',
    progress: 'fa-spinner fa-spin',
};

// Must match .notification.exiting's real animation-duration in
// NotificationStack.css - this is how long a dismissed notification's
// element is kept in the DOM after it leaves the store, so its exit
// animation actually gets to play instead of the item just vanishing.
const EXIT_MS = 200;

type DisplayItem = Notification & { exiting: boolean };

// Renders every active notification, newest last (so a freshly-added one
// appears at the bottom of the stack, right above wherever the next one
// will land) - one instance of this lives in app.tsx, right below TopBar.
//
// The store itself (data/notifications.ts) just drops a dismissed
// notification from its list immediately - there's no "exiting" concept
// there, and there shouldn't be, since nothing else about a notification
// cares how its own removal is animated. That's purely a display concern,
// so it's handled here instead: this component keeps its own local copy
// of the list, and when an item disappears from the store, keeps
// rendering it (marked .exiting) for EXIT_MS so its CSS animation can
// finish before the element actually leaves the DOM.
export function NotificationStack() {
    const notifications = useNotifications();
    const [items, setItems] = useState<DisplayItem[]>([]);
    const timersRef = useRef(new Map<string, ReturnType<typeof setTimeout>>());

    useEffect(() => {
        setItems((prev) => {
            const current = new Map(notifications.map((n) => [n.id, n]));
            const next: DisplayItem[] = [];
            for (const item of prev) {
                const fresh = current.get(item.id);
                if (fresh) {
                    next.push({...fresh, exiting: false});
                } else if (item.exiting) {
                    // Already animating out from an earlier pass - leave
                    // it be, its own timer (below) will remove it.
                    next.push(item);
                } else {
                    // Just dropped from the store - start its exit
                    // animation and schedule the real removal.
                    next.push({...item, exiting: true});
                    const id = item.id;
                    clearTimeout(timersRef.current.get(id));
                    timersRef.current.set(id, setTimeout(() => {
                        timersRef.current.delete(id);
                        setItems((cur) => cur.filter((x) => x.id !== id));
                    }, EXIT_MS));
                }
            }
            const known = new Set(next.map((item) => item.id));
            for (const n of notifications) {
                if (!known.has(n.id)) next.push({...n, exiting: false});
            }
            return next;
        });
    }, [notifications]);

    useEffect(() => () => {
        for (const timer of timersRef.current.values()) clearTimeout(timer);
    }, []);

    // Publishes where the bottom edge of the stack is (--notification-bottom, 0
    // when there is none), which the full-window overlays (.overlay and the
    // first-run wizard) use as their top padding: the stack is drawn above a
    // dialog's backdrop so a message raised from inside one is never hidden, and
    // this keeps the dialog laid out below it instead of underneath it. Followed
    // through a ResizeObserver so the dialog eases down and back up with the
    // messages' own enter and exit animations.
    const stackRef = useRef<HTMLDivElement>(null);
    const showing = items.length > 0;
    useEffect(() => {
        const root = document.documentElement.style;
        const el = stackRef.current;
        if (!showing || !el) {
            root.setProperty('--notification-bottom', '0px');
            return;
        }
        const publish = () => root.setProperty('--notification-bottom', `${el.getBoundingClientRect().bottom}px`);
        publish();
        const observer = new ResizeObserver(publish);
        observer.observe(el);
        return () => observer.disconnect();
    }, [showing]);

    if (items.length === 0) {
        return null;
    }
    // The stack floats over the page from the top bar's bottom edge instead of taking
    // room in the layout: the zero-height anchor sits in the flow right under the top
    // bar (so that is where the stack starts), and the stack is drawn over what is below.
    // A message must never push the view under it down and up again as it comes and goes.
    return (
        <div className="notification-anchor">
            <div className="notification-stack" ref={stackRef}>
                {items.map((n) => (
                    <div key={n.id} className={`notification notification-${n.kind} ${n.exiting ? 'exiting' : ''}`}>
                        <i className={`fa-solid ${ICONS[n.kind]}`}/>
                        <div className="notification-body">
                            <span className="notification-message">{n.message}</span>
                            {n.kind === 'progress' && (
                                <div className="notification-progress-track">
                                    <div
                                        className={`notification-progress-fill ${n.progress === undefined ? 'indeterminate' : ''}`}
                                        style={n.progress !== undefined ? {width: `${Math.max(0, Math.min(100, n.progress))}%`} : undefined}
                                    />
                                </div>
                            )}
                        </div>
                        {n.action && <span className="notification-action" onClick={n.action.onClick}>{n.action.label}</span>}
                        {n.dismissLabel && <span className="notification-action" onClick={() => dismiss(n.id)}>{n.dismissLabel}</span>}
                        <i className="fa-solid fa-xmark notification-close" onClick={() => dismiss(n.id)}/>
                    </div>
                ))}
            </div>
        </div>
    );
}
