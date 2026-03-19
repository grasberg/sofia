import pino from 'pino';

// Determine environment
const isDevelopment = process.env.NODE_ENV !== 'production';

// Create logger instance
const logger = pino({
  level: process.env.LOG_LEVEL || (isDevelopment ? 'debug' : 'info'),
  transport: isDevelopment
    ? {
        target: 'pino-pretty',
        options: {
          colorize: true,
          translateTime: 'SYS:standard',
          ignore: 'pid,hostname',
        },
      }
    : undefined,
  // Base fields for all logs
  base: {
    env: process.env.NODE_ENV || 'development',
    service: 'sofia-ops-app',
  },
  // Redact sensitive fields
  redact: {
    paths: [
      'password',
      '*.password',
      '*.token',
      '*.secret',
      'authorization',
      'cookie',
      'req.headers.authorization',
      'req.headers.cookie',
    ],
    censor: '[REDACTED]',
  },
});

// Export the logger
export default logger;

// Optional: Export convenience methods
export const log = logger;