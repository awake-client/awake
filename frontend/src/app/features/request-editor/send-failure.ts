/** A friendly explanation of why a request could not be sent. */
export interface SendFailure {
  title: string;
  hint: string;
  /** The raw error, kept for the details disclosure. */
  details: string;
}

interface Rule {
  matches: RegExp;
  describe: (match: RegExpMatchArray) => Omit<SendFailure, 'details'>;
}

const RULES: Rule[] = [
  {
    matches: /lookup ([^\s:]+).*no such host/i,
    describe: ([, host]) => ({
      title: 'Host not found',
      hint: `Could not resolve ${host}. Check the URL, your internet connection or your VPN.`,
    }),
  },
  {
    matches: /network is unreachable|no route to host/i,
    describe: () => ({
      title: 'Network unreachable',
      hint: 'You seem to be offline. Check your internet connection.',
    }),
  },
  {
    matches: /connection refused/i,
    describe: () => ({
      title: 'Connection refused',
      hint: 'The server is not accepting connections. Is it running, and is the port right?',
    }),
  },
  {
    matches: /deadline exceeded|Client\.Timeout|i\/o timeout|timed out/i,
    describe: () => ({
      title: 'Request timed out',
      hint: 'The server took too long to respond.',
    }),
  },
  {
    matches: /x509|certificate|tls:/i,
    describe: () => ({
      title: 'Secure connection failed',
      hint: 'The server certificate could not be verified.',
    }),
  },
  {
    matches: /connection reset|EOF/,
    describe: () => ({
      title: 'Connection closed',
      hint: 'The server closed the connection before responding.',
    }),
  },
  {
    matches: /unsupported protocol scheme|missing protocol scheme|invalid URL|no Host in request URL/i,
    describe: () => ({
      title: 'Invalid URL',
      hint: 'Check that the URL starts with http:// or https:// and has a host.',
    }),
  },
  {
    matches: /context canceled/i,
    describe: () => ({
      title: 'Request cancelled',
      hint: 'The request was stopped before it completed.',
    }),
  },
];

export function describeSendFailure(message: string): SendFailure {
  for (const rule of RULES) {
    const match = message.match(rule.matches);
    if (match) {
      return { ...rule.describe(match), details: message };
    }
  }
  return {
    title: 'The request could not be sent',
    hint: message,
    details: '',
  };
}
