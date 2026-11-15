const dayMs = 24 * 60 * 60 * 1000;

export function overviewChartRange(now = new Date(), completion = true) {
  const today = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  const week = new Date(today.getTime() - today.getUTCDay() * dayMs);
  return {
    from: new Date(completion ? week.getTime() - 21 * dayMs : today.getTime() - 6 * dayMs)
      .toISOString()
      .slice(0, 10),
    to: today.toISOString().slice(0, 10),
  };
}

export function overviewChartData(buckets, to) {
  const today = new Date(`${to}T00:00:00Z`);
  const week = new Date(today.getTime() - today.getUTCDay() * dayMs);
  const counts = new Map(buckets.map((bucket) => [bucket.date, bucket]));
  const dateKey = (date) => date.toISOString().slice(0, 10);
  // Use civil dates for labels so the browser's timezone cannot shift UTC buckets.
  const labelDate = (date) =>
    new Date(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate(), 12);
  return {
    completed: Array.from({ length: 4 }, (_, index) => {
      const start = new Date(week.getTime() - (3 - index) * 7 * dayMs);
      let count = 0;
      for (let offset = 0; offset < 7; offset++) {
        count += counts.get(dateKey(new Date(start.getTime() + offset * dayMs)))?.completed ?? 0;
      }
      return { date: labelDate(start), count };
    }),
    created: Array.from({ length: 7 }, (_, index) => {
      const date = new Date(today.getTime() - (6 - index) * dayMs);
      return { date: labelDate(date), count: counts.get(dateKey(date))?.created ?? 0 };
    }),
  };
}
