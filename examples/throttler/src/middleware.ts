import { Env } from "env";

export type Middleware = (
  req: Request,
  env: Env,
  next: (req: Request, env: Env) => Promise<Response>
) => Promise<Response>;

export const exitMiddleware: Middleware = async (req, env, _) => {
  return proxy(req, env);
};

// Methods that typically have a body
const METHODS_WITH_BODY = ["POST", "PUT", "PATCH"] as const;

/**
 * Handles the proxy request and applies necessary transformations.
 * @param request - The original request to be proxied.
 * @param env - The environment configuration.
 * @returns A modified response from the upstream server.
 */
async function proxy(request: Request, env: Env): Promise<Response> {
  const url = new URL(request.url);
  const originalHostname = url.hostname;

  // Configure URL protocol and port
  url.protocol = env.USE_HTTPS ? "https:" : "http:";
  url.port = env.USE_HTTPS ? "443" : "80";
  url.host = env.UPSTREAM;

  // Prepare headers for the upstream request
  const modifiedHeaders = new Headers(request.headers);
  modifiedHeaders.set("Host", env.UPSTREAM);
  modifiedHeaders.set("Referer", `${url.protocol}//${originalHostname}`);

  // Prepare the proxy request
  const proxyRequest: RequestInit = {
    method: request.method,
    headers: modifiedHeaders,
  };

  if (METHODS_WITH_BODY.includes(request.method as any)) {
    proxyRequest.body = request.body;
  }

  // Make the upstream request
  const response = await fetch(url.href, proxyRequest);

  // Modify response headers
  const modifiedResponseHeaders = new Headers(response.headers);

  // Add CORS headers
  modifiedResponseHeaders.set("access-control-allow-origin", "*");
  modifiedResponseHeaders.set("access-control-allow-credentials", "true");

  // Return modified response
  return new Response(response.body, {
    status: response.status,
    headers: modifiedResponseHeaders,
  });
}
