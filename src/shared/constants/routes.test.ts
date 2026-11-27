import { pathTo, RoutePath } from './routes';

describe('pathTo', () => {
  it('подставляет параметры в шаблоны RoutePath', () => {
    expect(pathTo.title('t1')).toBe('/title/t1');
    expect(pathTo.titleEdit('t1')).toBe('/admin/titles/t1');
    expect(pathTo.watch('t1')).toBe('/watch/t1');
    expect(pathTo.watch('t1', 'e 2')).toBe('/watch/t1?ep=e%202');
  });

  it('статические пути не меняются', () => {
    expect(RoutePath.profile).toBe('/cabinet/profile');
    expect(RoutePath.notFound).toBe('*');
  });
});
