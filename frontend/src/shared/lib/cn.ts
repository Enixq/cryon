import clsx, { type ClassValue } from "clsx";

/** Объединение классов через clsx. */
export function cn(...inputs: ClassValue[]): string {
  return clsx(inputs);
}
