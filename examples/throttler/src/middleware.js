"use strict";
var __awaiter = (this && this.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};
var __generator = (this && this.__generator) || function (thisArg, body) {
    var _ = { label: 0, sent: function() { if (t[0] & 1) throw t[1]; return t[1]; }, trys: [], ops: [] }, f, y, t, g = Object.create((typeof Iterator === "function" ? Iterator : Object).prototype);
    return g.next = verb(0), g["throw"] = verb(1), g["return"] = verb(2), typeof Symbol === "function" && (g[Symbol.iterator] = function() { return this; }), g;
    function verb(n) { return function (v) { return step([n, v]); }; }
    function step(op) {
        if (f) throw new TypeError("Generator is already executing.");
        while (g && (g = 0, op[0] && (_ = 0)), _) try {
            if (f = 1, y && (t = op[0] & 2 ? y["return"] : op[0] ? y["throw"] || ((t = y["return"]) && t.call(y), 0) : y.next) && !(t = t.call(y, op[1])).done) return t;
            if (y = 0, t) op = [op[0] & 2, t.value];
            switch (op[0]) {
                case 0: case 1: t = op; break;
                case 4: _.label++; return { value: op[1], done: false };
                case 5: _.label++; y = op[1]; op = [0]; continue;
                case 7: op = _.ops.pop(); _.trys.pop(); continue;
                default:
                    if (!(t = _.trys, t = t.length > 0 && t[t.length - 1]) && (op[0] === 6 || op[0] === 2)) { _ = 0; continue; }
                    if (op[0] === 3 && (!t || (op[1] > t[0] && op[1] < t[3]))) { _.label = op[1]; break; }
                    if (op[0] === 6 && _.label < t[1]) { _.label = t[1]; t = op; break; }
                    if (t && _.label < t[2]) { _.label = t[2]; _.ops.push(op); break; }
                    if (t[2]) _.ops.pop();
                    _.trys.pop(); continue;
            }
            op = body.call(thisArg, _);
        } catch (e) { op = [6, e]; y = 0; } finally { f = t = 0; }
        if (op[0] & 5) throw op[1]; return { value: op[0] ? op[1] : void 0, done: true };
    }
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.exitMiddleware = exports.modifyRequestMiddleware = exports.jitterer = void 0;
var MiddlewareChain = /** @class */ (function () {
    function MiddlewareChain() {
        this.middlewares = [];
    }
    MiddlewareChain.prototype.use = function (middleware) {
        this.middlewares.push(middleware);
    };
    MiddlewareChain.prototype.execute = function (initialReq) {
        var _this = this;
        var index = -1;
        var runner = function (i, req) {
            if (i <= index) {
                return Promise.reject(new Error("next() called multiple times"));
            }
            index = i;
            var middleware = _this.middlewares[i];
            if (middleware) {
                return middleware(req, function (newReq) { return runner(i + 1, newReq); });
            }
            else {
                return Promise.resolve(new Response("No final handler", { status: 500 }));
            }
        };
        return runner(0, initialReq);
    };
    return MiddlewareChain;
}());
exports.default = MiddlewareChain;
// Example middlewares
var jitterer = function (req, next) { return __awaiter(void 0, void 0, void 0, function () {
    return __generator(this, function (_a) {
        console.log("Logger Middleware: Received request:", req.url);
        return [2 /*return*/, next(req)]; // Pass the request along unchanged
    });
}); };
exports.jitterer = jitterer;
var modifyRequestMiddleware = function (req, next) { return __awaiter(void 0, void 0, void 0, function () {
    return __generator(this, function (_a) {
        console.log("Modify Request Middleware: Adding query parameter...");
        //   const url = new URL(req.url);
        //   url.searchParams.set("middleware", "true");
        //   const modifiedReq = new Request(url.toString(), req); // Clone request with updated URL
        return [2 /*return*/, next(req)];
    });
}); };
exports.modifyRequestMiddleware = modifyRequestMiddleware;
var exitMiddleware = function (req, next) { return __awaiter(void 0, void 0, void 0, function () {
    return __generator(this, function (_a) {
        console.log("Final Middleware: Modifying response...");
        return [2 /*return*/, proxy(req)];
    });
}); };
exports.exitMiddleware = exitMiddleware;
var CONFIG = {
    UPSTREAM: "jsonplaceholder.typicode.com",
    USE_HTTPS: true,
    ENABLE_JITTER: true,
    JITTER_DELAY: 100,
    ENABLE_OBSERVER: true,
    ENABLE_BACKPRESSURE: true,
};
// Methods that typically have a body
var METHODS_WITH_BODY = ["POST", "PUT", "PATCH"];
/**
 * Handles the proxy request and applies necessary transformations
 * @param request Original request to be proxied
 * @returns Modified response
 */
function proxy(request) {
    return __awaiter(this, void 0, void 0, function () {
        var url, originalHostname, modifiedHeaders, proxyRequest, clonedRequest, contentType, body, formData, formData, body, error_1, response, modifiedResponseHeaders, hi;
        return __generator(this, function (_a) {
            switch (_a.label) {
                case 0:
                    url = new URL(request.url);
                    originalHostname = url.hostname;
                    // Configure URL protocol and port
                    url.protocol = CONFIG.USE_HTTPS ? "https:" : "http:";
                    url.port = CONFIG.USE_HTTPS ? "443" : "80";
                    url.host = CONFIG.UPSTREAM;
                    modifiedHeaders = new Headers(request.headers);
                    modifiedHeaders.set("Host", CONFIG.UPSTREAM);
                    modifiedHeaders.set("Referer", "".concat(url.protocol, "//").concat(originalHostname));
                    proxyRequest = {
                        method: request.method,
                        headers: modifiedHeaders,
                    };
                    console.log("got here did ya");
                    if (!METHODS_WITH_BODY.includes(request.method)) return [3 /*break*/, 11];
                    _a.label = 1;
                case 1:
                    _a.trys.push([1, 10, , 11]);
                    clonedRequest = request.clone();
                    contentType = request.headers.get("content-type");
                    if (!(contentType === null || contentType === void 0 ? void 0 : contentType.includes("application/json"))) return [3 /*break*/, 3];
                    return [4 /*yield*/, clonedRequest.json()];
                case 2:
                    body = _a.sent();
                    proxyRequest.body = JSON.stringify(body);
                    return [3 /*break*/, 9];
                case 3:
                    if (!(contentType === null || contentType === void 0 ? void 0 : contentType.includes("application/x-www-form-urlencoded"))) return [3 /*break*/, 5];
                    return [4 /*yield*/, clonedRequest.formData()];
                case 4:
                    formData = _a.sent();
                    proxyRequest.body = new URLSearchParams(formData).toString();
                    return [3 /*break*/, 9];
                case 5:
                    if (!(contentType === null || contentType === void 0 ? void 0 : contentType.includes("multipart/form-data"))) return [3 /*break*/, 7];
                    return [4 /*yield*/, clonedRequest.formData()];
                case 6:
                    formData = _a.sent();
                    proxyRequest.body = formData;
                    return [3 /*break*/, 9];
                case 7: return [4 /*yield*/, clonedRequest.blob()];
                case 8:
                    body = _a.sent();
                    proxyRequest.body = body;
                    _a.label = 9;
                case 9: return [3 /*break*/, 11];
                case 10:
                    error_1 = _a.sent();
                    // If body parsing fails, forward the original request body
                    proxyRequest.body = request.body;
                    return [3 /*break*/, 11];
                case 11: return [4 /*yield*/, fetch(url.href, proxyRequest)];
                case 12:
                    response = _a.sent();
                    modifiedResponseHeaders = new Headers(response.headers);
                    // Add CORS headers
                    modifiedResponseHeaders.set("access-control-allow-origin", "*");
                    modifiedResponseHeaders.set("access-control-allow-credentials", "true");
                    hi = new Response(response.body, {
                        status: response.status,
                        headers: modifiedResponseHeaders,
                    });
                    return [2 /*return*/, hi];
            }
        });
    });
}
