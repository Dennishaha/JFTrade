import surfaces from "./capability-surfaces.json";

export type CapabilitySurfaceID = keyof typeof surfaces;

export function capabilitySurfaceID<T extends CapabilitySurfaceID>(id: T): T {
  return id;
}

export function isCapabilitySurfaceID(
  value: string,
): value is CapabilitySurfaceID {
  return Object.prototype.hasOwnProperty.call(surfaces, value);
}

export const capabilitySurfaceManifest = surfaces;
