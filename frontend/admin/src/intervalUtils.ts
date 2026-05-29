export type IntervalDraft = { min: number; max: number };

export function intervalError(draft: IntervalDraft): string {
  if (!draft.min || !draft.max) return "Both values required";
  if (draft.min <= 0 || draft.max <= 0) return "Values must be positive";
  if (draft.min > draft.max) return "Min cannot be greater than max";
  return "";
}

export function intervalValid(draft: IntervalDraft): boolean {
  return intervalError(draft) === "";
}
