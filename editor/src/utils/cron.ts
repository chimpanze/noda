/**
 * Convert a 5-field cron expression to a human-readable description.
 * Handles common patterns; falls back to the raw expression for complex ones.
 */
export function describeCron(expr: string): string {
  const parts = expr.trim().split(/\s+/);
  if (parts.length !== 5) return expr;

  const [minute, hour, dom, month, dow] = parts;

  // Every minute
  if (
    minute === "*" &&
    hour === "*" &&
    dom === "*" &&
    month === "*" &&
    dow === "*"
  ) {
    return "Every minute";
  }

  // Every N minutes
  if (
    minute.startsWith("*/") &&
    hour === "*" &&
    dom === "*" &&
    month === "*" &&
    dow === "*"
  ) {
    const n = minute.slice(2);
    return `Every ${n} minute${n === "1" ? "" : "s"}`;
  }

  // Every N hours
  if (
    minute === "0" &&
    hour.startsWith("*/") &&
    dom === "*" &&
    month === "*" &&
    dow === "*"
  ) {
    const n = hour.slice(2);
    return `Every ${n} hour${n === "1" ? "" : "s"}`;
  }

  // Specific time daily
  if (
    /^\d+$/.test(minute) &&
    /^\d+$/.test(hour) &&
    dom === "*" &&
    month === "*" &&
    dow === "*"
  ) {
    return `Daily at ${pad(hour)}:${pad(minute)}`;
  }

  // Specific time on certain days of week
  if (
    /^\d+$/.test(minute) &&
    /^\d+$/.test(hour) &&
    dom === "*" &&
    month === "*" &&
    dow !== "*"
  ) {
    const days = parseDow(dow);
    return `${days} at ${pad(hour)}:${pad(minute)}`;
  }

  // Top of every hour
  if (
    minute === "0" &&
    hour === "*" &&
    dom === "*" &&
    month === "*" &&
    dow === "*"
  ) {
    return "Every hour";
  }

  // At specific minute every hour
  if (
    /^\d+$/.test(minute) &&
    hour === "*" &&
    dom === "*" &&
    month === "*" &&
    dow === "*"
  ) {
    return `Every hour at :${pad(minute)}`;
  }

  return expr;
}

const DOW_NAMES = [
  "Sunday",
  "Monday",
  "Tuesday",
  "Wednesday",
  "Thursday",
  "Friday",
  "Saturday",
];

function parseDow(dow: string): string {
  const nums = dow.split(",").map((s) => {
    const n = parseInt(s.trim(), 10);
    return DOW_NAMES[n] ?? s;
  });
  if (nums.length === 1) return `Every ${nums[0]}`;
  return nums.join(", ");
}

function pad(s: string): string {
  return s.padStart(2, "0");
}
