import { Middleware, exitMiddleware } from "middleware";
import { jitterer } from "jitterer";
import { backpressure } from "backpressure";
import { Env } from "env";
export { Counter } from "durable_counter";

export default {
  async fetch(req: Request, env: Env): Promise<Response> {
    const chain = new MiddlewareChain();
    chain.use(jitterer, env.ENABLE_JITTER);
    chain.use(backpressure, env.BACKPRESSURE.ENABLE_BACKPRESSURE);
    chain.use(exitMiddleware);

    try {
      return chain.execute(req, env);
    } catch (error) {
      return new Response("Internal Server Error", { status: 500 });
    }
  },
} satisfies ExportedHandler<Env>;

class MiddlewareChain {
  private middlewares: Middleware[] = [];

  use(middleware: Middleware, enabled: boolean = true): void {
    if (enabled) {
      this.middlewares.push(middleware);
    }
  }

  execute(initialReq: Request, env: Env): Promise<Response> {
    let index = -1;

    const runner = (i: number, req: Request): Promise<Response> => {
      if (i <= index) {
        return Promise.reject(new Error("next() called multiple times"));
      }
      index = i;
      const middleware = this.middlewares[i];
      if (middleware) {
        return middleware(req, env, (newReq) => runner(i + 1, newReq));
      } else {
        return Promise.resolve(
          new Response("No final handler", { status: 500 })
        );
      }
    };

    return runner(0, initialReq);
  }
}
