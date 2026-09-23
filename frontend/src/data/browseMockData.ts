// Fields the redesigned Browse detail view shows that internal/loverslab does not extract yet -
// confirmed real and present on the live site (tags, a Followers count, a Likes/reaction count,
// separate Submitted/Published dates, a Requirements list, and richer per-comment metadata), just
// not parsed into loverslab.FileSummary/FileDetail/Post today. Everything else Browse shows (the
// file grid, the description, screenshots, Views/Downloads, comment text) stays real - only these
// specific extras are mocked for now, joined onto a real item by its own LoversLab ID so the same
// file or comment always gets the same mock values across reloads and pagination, not a new
// random pick every render. None of the pool entries below are copied from any real LoversLab
// page - invented example text only. Delete this file once internal/loverslab extracts the real
// fields and Browse.tsx reads them directly instead (see docs/loverslab.md).

export interface MockRequirement {
    name: string;
    installed: boolean;
}

export interface MockFileExtras {
    tag: string;
    tagColor: string;
    tags: string[];
    followers: string;
    likes: number;
    submitted: string;
    published: string;
    requirements: MockRequirement[];
    features: string[];
}

export interface MockCommentExtras {
    authorGroup: string;
    authorPostCount: number;
    authorTitle?: string;
    isPopular: boolean;
    reactions: number;
    edited: boolean;
}

function hashIndex(seed: string, poolSize: number): number {
    let h = 0;
    for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) | 0;
    return Math.abs(h) % poolSize;
}

const TAG_POOL: {tag: string; tagColor: string; tags: string[]}[] = [
    {tag: 'SPECIES', tagColor: '#8fb8d8', tags: ['portraits', 'species', 'graphics']},
    {tag: 'EVENTS', tagColor: '#c9a27f', tags: ['events', 'story', 'roleplay']},
    {tag: 'BUILDINGS', tagColor: '#9fd8a0', tags: ['buildings', 'economy', 'districts']},
    {tag: 'FRAMEWORK', tagColor: '#d8b48f', tags: ['framework', 'traits', 'civics']},
    {tag: 'UI', tagColor: '#a98bdc', tags: ['interface', 'quality of life']},
];

const FOLLOWER_POOL = ['312', '1.2k', '48', '2.4k', '89'];
const LIKE_POOL = [72, 449, 15, 203, 31];
const SUBMITTED_POOL = ['March 3, 2021', 'August 20, 2019', 'June 14, 2023', 'January 2, 2022', 'November 9, 2020'];
const PUBLISHED_POOL = ['March 3, 2021', 'August 20', 'June 15, 2023', 'January 4, 2022', 'November 10, 2020'];

const REQUIREMENT_POOL: MockRequirement[][] = [
    [{name: 'Portrait Framework Core', installed: true}],
    [{name: 'UI Overhaul Dynamic', installed: false}],
    [],
    [{name: 'Ethics and Civics Expanded', installed: true}, {name: 'Ethics and Civics Expanded - Portraits', installed: false}],
];

const FEATURES_POOL: string[][] = [
    ['212 new portraits, each with 4 animation states', 'Optional clothing packs per ethic', 'Localised names in EN, DE, FR, RU'],
    ['Adds 40 new mid-game events', 'Fully voiced narration for the main event chain', 'Compatible with most overhaul mods'],
    ['New building line for every ethic', 'Balanced against vanilla costs', 'No hard dependency on any DLC'],
    [],
];

// mockExtrasFor derives a stable set of the fields above for a real file, keyed by its own
// LoversLab file ID.
export function mockExtrasFor(fileID: number): MockFileExtras {
    const seed = String(fileID);
    const tagPick = TAG_POOL[hashIndex(seed, TAG_POOL.length)];
    return {
        tag: tagPick.tag,
        tagColor: tagPick.tagColor,
        tags: tagPick.tags,
        followers: FOLLOWER_POOL[hashIndex(seed + 'f', FOLLOWER_POOL.length)],
        likes: LIKE_POOL[hashIndex(seed + 'l', LIKE_POOL.length)],
        submitted: SUBMITTED_POOL[hashIndex(seed + 's', SUBMITTED_POOL.length)],
        published: PUBLISHED_POOL[hashIndex(seed + 'p', PUBLISHED_POOL.length)],
        requirements: REQUIREMENT_POOL[hashIndex(seed + 'r', REQUIREMENT_POOL.length)],
        features: FEATURES_POOL[hashIndex(seed + 'ft', FEATURES_POOL.length)],
    };
}

const GROUP_POOL = ['Members', 'Advanced Member', 'Community Team'];
const TITLE_POOL: (string | undefined)[] = [undefined, 'Snuggle Butt Princess', 'Perpetually Tired Modder', undefined, 'Definitely Not a Bot'];

// mockCommentExtrasFor derives a stable set of per-comment fields for a real post, keyed by its
// own comment ID. Whether a post is the topic author's own (for the "Topic Author" badge) is
// real data now, not mock - LoversLabCommentList.TopicAuthor - so it isn't part of this.
export function mockCommentExtrasFor(commentID: string): MockCommentExtras {
    const seed = commentID || 'x';
    return {
        authorGroup: GROUP_POOL[hashIndex(seed + 'g', GROUP_POOL.length)],
        authorPostCount: 40 + hashIndex(seed + 'pc', 900),
        authorTitle: TITLE_POOL[hashIndex(seed + 't', TITLE_POOL.length)],
        isPopular: hashIndex(seed + 'pop', 5) === 0,
        reactions: hashIndex(seed + 'r', 180),
        edited: hashIndex(seed + 'e', 6) === 0,
    };
}
