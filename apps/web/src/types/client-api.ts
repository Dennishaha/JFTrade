import type { ApiEnvelopeDto, ApiErrorEnvelopeDto } from "@/contracts";

type GeneratedEnvelope = ApiEnvelopeDto;
type GeneratedErrorEnvelope = ApiErrorEnvelopeDto;

export type ApiSuccessEnvelope<T> = Omit<
  GeneratedEnvelope,
  "data" | "error" | "ok"
> & {
  ok: true;
  data: T;
  error?: never;
};

export type ApiErrorEnvelope = Omit<GeneratedErrorEnvelope, "ok"> & {
  ok: false;
};
