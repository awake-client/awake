// HMAC Signer: an example Awake plugin. It runs in Awake's JavaScript runtime,
// without modules, file system or network: everything goes through `awake`.

// The text the signature covers. The request tab shows it, so it is computed
// in one place.
function canonical(method, url, body) {
  return [method.toUpperCase(), url, awake.crypto.sha256(body || "")].join("\n");
}

function randomHex(length) {
  let text = "";
  while (text.length < length) {
    text += Math.floor(Math.random() * 16).toString(16);
  }
  return text;
}

awake.register({
  authentication: {
    // Runs on the final request, just before it is sent.
    hmac(context) {
      const secret = context.settings.secret;
      if (!secret) {
        throw new Error("set the signing secret in Plugins → HMAC Signer → Settings");
      }
      const request = context.request;
      const signature = awake.crypto.hmacSha256(secret, canonical(request.method, request.url, request.body));
      const scheme = context.settings.algorithmLabel;
      request.headers[context.settings.header] = `${scheme} keyId=${context.fields.keyId},signature=${signature}`;
    },
  },

  dynamicVariables: {
    $requestId: () => randomHex(16),
    $isoDate: () => new Date().toISOString(),
  },

  // Runs before the pre-request script of every request of the workspace.
  beforeRequest(context) {
    if (!("X-Request-Id" in context.request.headers)) {
      context.request.headers["X-Request-Id"] = randomHex(16);
    }
  },

  commands: {
    "show-settings"(context) {
      awake.log("header:", context.settings.header);
      awake.log("scheme:", context.settings.algorithmLabel);
      awake.log("secret:", context.settings.secret ? "set on this computer" : "not set");
    },
  },

  importFormats: {
    // One request per non-empty line; "METHOD URL" or just "URL".
    urls(context) {
      const requests = context.content
        .split("\n")
        .map((line) => line.trim())
        .filter((line) => line !== "" && !line.startsWith("#"))
        .map((line, index) => {
          const parts = line.split(/\s+/);
          const method = parts.length > 1 ? parts[0].toUpperCase() : "GET";
          const url = parts.length > 1 ? parts[1] : parts[0];
          return { name: `Request ${index + 1}`, request: { method, url } };
        });
      return [{ name: context.fileName.replace(/\.[^.]+$/, ""), requests }];
    },
  },

  exportFormats: {
    urls(context) {
      const lines = [];
      const visit = (requests, folders) => {
        for (const entry of requests || []) {
          lines.push(`${entry.request.method} ${entry.request.url}`);
        }
        for (const folder of folders || []) {
          visit(folder.requests, folder.folders);
        }
      };
      for (const collection of context.collections) {
        lines.push(`# ${collection.name}`);
        visit(collection.requests, collection.folders);
      }
      return lines.join("\n") + "\n";
    },
  },

  // Handlers the pages of the views may call with awake:invoke.
  views: {
    signature: {
      canonical(context) {
        const request = context.payload.request;
        return {
          header: context.settings.header,
          text: canonical(request.Method || "GET", request.URL || "", request.Body ? request.Body.Content : ""),
        };
      },
    },
  },
});
