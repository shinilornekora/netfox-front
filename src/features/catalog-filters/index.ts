// Публичный API фичи «Фильтры витрины».
export { CatalogFilters } from './ui/CatalogFilters';
export { catalogFiltersSlice, filterChanged, filtersCleared, initialFilters, selectFilters } from './model';
export type { Filters } from './model';
