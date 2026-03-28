export type ProbeRecord = {
  id: string;
  createdAt: string;
  stationName: string;
  groupName: string | null;
  baseUrl: string;
  apiKeyHash: string;
  apiKeyMasked: string;
  claimedChannel: string | null;
  expectedModelFamily: string | null;
  status: string;
  ruleBasedScore: number;
  ruleBasedVerdict: "trusted" | "needs_review" | "high_risk";
  trustScore: number;
  verdict: "trusted" | "needs_review" | "high_risk";
  httpStatus: number | null;
  detectedEndpoint: string | null;
  responseTimeMs: number | null;
  isOpenAiCompatible: boolean;
  primaryFamily: string | null;
  detectedFamilies: string[];
  modelIds: string[];
  responseHeaders: Record<string, string>;
  suspicionReasons: string[];
  notes: string[];
  channelScore: number | null;
  channelVerdict: "trusted" | "needs_review" | "high_risk" | null;
  channelConfidence: number | null;
  channelSummary: string | null;
  channelSupportingSignals: string[];
  channelRiskSignals: string[];
  channelMissingEvidence: string[];
  channelConsistency: {
    claimedChannelMatchesModelPool: boolean;
    claimedChannelMatchesEndpointBehavior: boolean;
    claimedChannelMatchesErrorStyle: boolean;
    isLikelyGenericOpenAIWrapper: boolean;
    isLikelyMixedProviderPool: boolean;
  } | null;
  channelReasoning: {
    modelPoolAssessment: string;
    endpointAssessment: string;
    errorStyleAssessment: string;
    finalAssessment: string;
  } | null;
  channelAuditModel: string | null;
  channelAuditError: string | null;
  errorMessage: string | null;
  rawExcerpt: string | null;
};

export type ProbeResponse = {
  probe: ProbeRecord;
  summary: {
    verdict: ProbeRecord["verdict"];
    trustScore: number;
    primaryFamily: string | null;
    suspicious: boolean;
  };
};

export type ProbeListResponse = {
  items: ProbeRecord[];
};

export type RankingItem = {
  name: string;
  totalProbes: number;
  avgScore: number;
  successRate: number;
  highRiskCount: number;
  lastProbeAt: string;
};

export type RankingResponse = {
  red: RankingItem[];
  black: RankingItem[];
};

export type ProbeForm = {
  stationName: string;
  groupName: string;
  baseUrl: string;
  apiKey: string;
  claimedChannel: string;
  expectedModelFamily: string;
};

export type ChannelModelMapResponse = {
  channels: Record<string, string[]>;
};

export type ChannelModelEntry = {
  id: number;
  channelName: string;
  modelId: string;
  isEnabled: boolean;
  createdAt: string;
  updatedAt: string;
};

export type ChannelModelListResponse = {
  items: ChannelModelEntry[];
};

export type ChannelModelUpsertRequest = {
  channelName: string;
  modelId: string;
  isEnabled: boolean;
};

export type ProbeManualUpdateRequest = {
  claimedChannel: string;
  expectedModelFamily: string;
  status: "success" | "auth_failed" | "invalid_response" | "request_failed";
  verdict: ProbeRecord["verdict"];
  trustScore: number;
  primaryFamily: string;
  modelIds: string[];
  suspicionReasons: string[];
  notes: string[];
};

export type AdminUserProfile = {
  id: number;
  username: string;
  createdAt: string;
  updatedAt: string;
  lastLoginAt: string | null;
};

export type AdminSessionResponse = {
  configured: boolean;
  authenticated: boolean;
  user: AdminUserProfile | null;
};

export type AdminLoginRequest = {
  username: string;
  password: string;
};

export type AdminAccountUpdateRequest = {
  username: string;
  currentPassword: string;
  newPassword: string;
};

export type SystemSettingsResponse = {
  channelAuditEnabled: boolean;
  channelAuditTimeoutMs: number;
  openAiApiKeyMasked: string;
  openAiApiKeyConfigured: boolean;
  openAiModel: string;
  openAiBaseUrl: string;
};

export type SystemSettingsUpdateRequest = {
  channelAuditEnabled: boolean;
  channelAuditTimeoutMs: number;
  openAiApiKey: string;
  clearOpenAiApiKey: boolean;
  openAiModel: string;
  openAiBaseUrl: string;
};
