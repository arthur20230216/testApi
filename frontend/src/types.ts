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
