import { CriticalityDefault } from "criticality";

export const HeaderCriticality = "X-Request-Criticality";
export const HeaderCanWait = "X-Can-Wait";
const HeaderDefaults = {
  HeaderCriticality: CriticalityDefault,
};

export function parseHeaderKey(req: Request, key: string): string {
  const headerValue = req.headers.get(key);
  if (!headerValue && key in HeaderDefaults) {
    return HeaderDefaults[key];
  }
  return headerValue ? headerValue : "";
}
