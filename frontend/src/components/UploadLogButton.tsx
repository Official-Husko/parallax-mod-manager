import {h} from 'preact';
import {tip} from '../data/tooltip';
import {TipItem} from './Tooltip';

// The log views' "Upload" button, shared by the activity log and the game log.
// Not built yet: it is drawn disabled, with no handler, to reserve the spot and
// make the intent visible - what it will do is in docs/log-sharing.md. Wiring it
// up means taking an onUpload prop here (like the log view's onOpenFolder) and
// dropping the "inert" class; the styling for both states exists.
export function UploadLogButton() {
    return (
        <span
            className="log-btn inert"
            {...tip(() => (
                <TipItem icon="fa-cloud-arrow-up" color="var(--blue)" title="Upload log - coming soon">
                    Share this log to help investigate a problem. It will show exactly what would be
                    sent, with an option to include computer details, and ask before anything leaves
                    this computer.
                </TipItem>
            ))}
        >
            <i className="fa-solid fa-cloud-arrow-up"/> Upload
        </span>
    );
}
