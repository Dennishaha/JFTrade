export interface WatchlistGroup {
  id: string;
  name: string;
  isDefault?: boolean;
  protected?: boolean;
  revision: number;
  itemCount?: number;
  createdAt?: string;
  updatedAt?: string;
}

export interface WatchlistItemSource {
  sourceId: string;
  sourceName?: string;
  remoteGroupName?: string;
  importedAt?: string;
}

export interface WatchlistItem {
  instrumentId: string;
  market: string;
  symbol: string;
  name?: string;
  securityType?: string;
  groupIds?: string[];
  groupNames?: string[];
  sources?: WatchlistItemSource[];
  createdAt?: string;
  updatedAt?: string;
}

export interface WatchlistItemsPage {
  items: WatchlistItem[];
  nextCursor?: string | null;
  total?: number;
}

export interface WatchlistMembership {
  instrumentId: string;
  groupIds: string[];
  groups?: Array<{ id: string; name: string }>;
  revision: number;
}

export interface WatchlistMembershipUpdate {
  groupIds: string[];
  newGroupNames: string[];
  expectedRevision: number;
}

export interface WatchlistExtendedQuote {
  price?: number;
  change?: number;
  changePercent?: number;
  observedAt?: string;
}

export interface WatchlistQuote {
  instrumentId: string;
  name?: string;
  securityType?: string;
  price?: number;
  previousClose?: number;
  change?: number;
  changePercent?: number;
  session?: string;
  observedAt?: string;
  updateTime?: string;
  source?: string;
  preMarket?: WatchlistExtendedQuote;
  afterHours?: WatchlistExtendedQuote;
  overnight?: WatchlistExtendedQuote;
}

export interface WatchlistQuoteError {
  instrumentId: string;
  code?: string;
  message: string;
}

export interface WatchlistQuoteBatch {
  quotes: WatchlistQuote[];
  errors: WatchlistQuoteError[];
  observedAt: string;
}

export interface WatchlistSource {
  id: string;
  broker?: string;
  displayName: string;
  available: boolean;
  status?: string;
  message?: string;
}

export interface WatchlistRemoteGroup {
  remoteGroupId: string;
  name: string;
  type?: string;
  system?: boolean;
  ambiguous?: boolean;
  memberCount?: number;
}

export interface WatchlistBinding {
  id: string;
  sourceId: string;
  remoteGroupId: string;
  remoteGroupName: string;
  localGroupId: string;
  localGroupName?: string;
  lastImportedAt?: string;
}

export interface WatchlistImportDiffItem {
  instrumentId: string;
  name?: string;
  selected?: boolean;
}

export interface WatchlistImportPreviewRequest {
  sourceId: string;
  remoteGroupId: string;
  remoteGroupName?: string;
  localGroupId?: string;
  newGroupName?: string;
}

export interface WatchlistImportPreview {
  id: string;
  sourceId: string;
  remoteGroupName: string;
  localGroupId?: string;
  localGroupName?: string;
  added: WatchlistImportDiffItem[];
  unchanged: WatchlistImportDiffItem[];
  localOnly: WatchlistImportDiffItem[];
  expiresAt?: string;
  remoteHash?: string;
  localRevision?: number;
}

export interface WatchlistImportCommitRequest {
  deleteLocalOnlyInstrumentIds: string[];
}

export interface WatchlistImportRun {
  id: string;
  previewId?: string;
  sourceId: string;
  remoteGroupName?: string;
  localGroupId?: string;
  localGroupName?: string;
  addedCount?: number;
  deletedCount?: number;
  unchangedCount?: number;
  status: string;
  createdAt?: string;
  completedAt?: string;
}
