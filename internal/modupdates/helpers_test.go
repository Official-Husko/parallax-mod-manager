package modupdates

import "time"

func (f Fingerprint) newestTime() time.Time { return time.Unix(f.Newest, 0) }
