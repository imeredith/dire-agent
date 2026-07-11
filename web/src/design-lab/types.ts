import type { ComponentType, LazyExoticComponent } from "react";

export interface DesignConcept {
  id: string;
  number: string;
  name: string;
  shortName: string;
  library: string;
  thesis: string;
  bestFor: string;
  accent: string;
  Component: LazyExoticComponent<ComponentType>;
}
