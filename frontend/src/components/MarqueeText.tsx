import './MarqueeText.css';
import {h} from 'preact';
import {useEffect, useRef, useState} from 'preact/hooks';

// Renders text that clips (never forces its container into a horizontal
// scrollbar) instead of overflowing, but reveals the full real text via a
// slow, looping horizontal scroll on hover - a mod name or file path is
// real, user-authored content, so hiding the rest of it behind an
// ellipsis forever with no way to read it isn't good enough on its own.
//
// Whether it's actually truncated is measured for real via ResizeObserver
// (comparing the text's own scrollWidth against its container's
// clientWidth) rather than guessed from string length - the same text
// fits or doesn't depending entirely on the container's own width, which
// this component doesn't control and can change (a window resize, a
// column reflow).
export function MarqueeText({text, className}: { text: string; className?: string }) {
    const containerRef = useRef<HTMLSpanElement>(null);
    const textRef = useRef<HTMLSpanElement>(null);
    const [overflowing, setOverflowing] = useState(false);

    useEffect(() => {
        const container = containerRef.current;
        const textEl = textRef.current;
        if (!container || !textEl) return;
        const check = () => setOverflowing(textEl.scrollWidth > container.clientWidth + 1);
        check();
        const observer = new ResizeObserver(check);
        observer.observe(container);
        return () => observer.disconnect();
    }, [text]);

    return (
        <span ref={containerRef} className={`marquee ${overflowing ? 'overflowing' : ''} ${className ?? ''}`}>
            <span className="marquee-track">
                <span ref={textRef} className="marquee-item">{text}</span>
                {overflowing && <span className="marquee-item" aria-hidden="true">{text}</span>}
            </span>
        </span>
    );
}
