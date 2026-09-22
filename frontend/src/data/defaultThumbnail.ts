import {DefaultThumbnail} from '../../wailsjs/go/main/App';

// The app's own placeholder art (see App.DefaultThumbnail - the same image a newly generated
// patch mod gets as its thumbnail), for a "no preview available" spot elsewhere in the interface.
// Fetched once and shared for the rest of the session: it never changes, and is a real piece of
// art rather than an icon, too large to fetch again every time something that needs it mounts.
let cached: Promise<string> | null = null;

export function defaultThumbnail(): Promise<string> {
    if (!cached) cached = DefaultThumbnail().catch(() => '');
    return cached;
}
