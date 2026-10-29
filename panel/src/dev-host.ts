if (import.meta.env.DEV) {
  const query = new URLSearchParams(location.search);
  const token = query.get('t');
  if (token) sessionStorage.setItem('qd.token', token);
  const asked = query.get('phone');
  if (asked !== null) sessionStorage.setItem('qd.phone', asked === '0' ? '' : '1');
  if (sessionStorage.getItem('qd.phone'))
    (window as unknown as { qdHost: object }).qdHost = {
      paint() {},
      checks: () => '[]',
      insets: () => (matchMedia('(orientation: portrait)').matches ? '39,16,0,0,16' : '24,16,39,0,16'),
    };
}

export {};
