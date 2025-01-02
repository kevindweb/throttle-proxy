import { Middleware, exitMiddleware } from "middleware";
import { jitterer } from "jitterer";
import { backpressure } from "backpressure";
import { Env } from "env";
export { Counter } from "durable_counter";

export default {
  async fetch(req: Request, env: Env): Promise<Response> {
    const chain = new MiddlewareChain();
    chain.use(jitterer, env.ENABLE_JITTER);
    chain.use(backpressure, env.BACKPRESSURE.ENABLED);
    chain.use(exitMiddleware);

    try {
      return chain.execute(req, env);
    } catch (error) {
      console.error("Error during middleware execution:", error);
      return new Response("Internal Server Error", { status: 500 });
    }
  },
  async tail(events) {
    console.log(`did this run ${JSON.stringify(events)}`);
  },
} satisfies ExportedHandler<Env>;

class MiddlewareChain {
  private middlewares: Middleware[] = [];

  /**
   * Adds a middleware to the chain if enabled.
   * @param middleware - The middleware function to add.
   * @param enabled - Whether the middleware should be added.
   */
  use(middleware: Middleware, enabled: boolean = true): void {
    if (enabled) {
      this.middlewares.push(middleware);
    }
  }

  /**
   * Executes the middleware chain.
   * @param initialReq - The initial request to process.
   * @param env - The environment variables.
   * @returns A promise resolving to a Response object.
   */
  async execute(initialReq: Request, env: Env): Promise<Response> {
    let index = -1;

    const runner = async (i: number, req: Request): Promise<Response> => {
      if (i <= index) {
        throw new Error("next() called multiple times");
      }
      index = i;

      const middleware = this.middlewares[i];
      if (middleware) {
        return middleware(req, env, (newReq) => runner(i + 1, newReq));
      }

      // Default case if no middleware returns a response.
      return new Response("No final handler", { status: 500 });
    };

    return runner(0, initialReq);
  }
}
