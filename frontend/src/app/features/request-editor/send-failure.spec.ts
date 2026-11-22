import { describeSendFailure } from './send-failure';

describe('describeSendFailure', () => {
  it('explains an unknown host', () => {
    const raw =
      'send request: Get "https://api.themoviedb.org/3/movie/1": dial tcp: lookup api.themoviedb.org: no such host';
    const failure = describeSendFailure(raw);
    expect(failure.title).toBe('Host not found');
    expect(failure.hint).toContain('api.themoviedb.org');
    expect(failure.details).toBe(raw);
  });

  it('explains a refused connection', () => {
    expect(
      describeSendFailure('send request: Get "http://localhost:3000": dial tcp [::1]:3000: connect: connection refused')
        .title,
    ).toBe('Connection refused');
  });

  it('explains a timeout', () => {
    expect(describeSendFailure('send request: context deadline exceeded').title).toBe('Request timed out');
  });

  it('falls back to the raw message', () => {
    const failure = describeSendFailure('something odd');
    expect(failure.hint).toBe('something odd');
    expect(failure.details).toBe('');
  });
});
