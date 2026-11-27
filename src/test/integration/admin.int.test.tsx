import { describe, expect, it } from 'vitest';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderApp } from '@/test/render';
import { db } from '@/test/gateway';

describe('администратор: каталог и продажи', () => {
  it('заводит сериал, добавляет серию, публикует — и он на витрине', async () => {
    const user = userEvent.setup();
    renderApp('/admin/titles', { as: 'admin' });
    await user.click(await screen.findByRole('link', { name: '+ Новый тайтл' }));
    const form = await screen.findByRole('form', { name: 'Карточка тайтла' });
    await user.selectOptions(within(form).getByLabelText('Тип'), 'series');
    await user.type(within(form).getByLabelText('Название'), 'Вампиры');
    await user.clear(within(form).getByLabelText('Год'));
    await user.type(within(form).getByLabelText('Год'), '1915');
    await user.click(within(form).getByRole('button', { name: 'детектив' }));
    await user.type(within(form).getByLabelText('Описание'), 'Репортёр охотится за бандой «Вампиров».');
    expect(within(form).queryByLabelText(/Цена покупки/)).not.toBeInTheDocument();
    await user.click(within(form).getByRole('button', { name: 'Создать черновик' }));

    expect(await screen.findByRole('heading', { name: 'Редактирование: Вампиры' })).toBeInTheDocument();
    expect(screen.getByText('Черновик')).toBeInTheDocument();
    const epForm = screen.getByRole('form', { name: 'Новая серия' });
    await user.type(within(epForm).getByLabelText('Название серии'), 'Отрубленная голова');
    await user.click(within(epForm).getByRole('button', { name: 'Добавить серию' }));
    expect(await screen.findByRole('heading', { name: 'Серии (1)' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Опубликовать' }));
    expect(await screen.findByText('Опубликовано — тайтл появился на витрине')).toBeInTheDocument();
    const t = db.titles.find((x) => x.name === 'Вампиры')!;
    expect(t).toMatchObject({ status: 'published', kind: 'series', genres: ['детектив'], episode_count: 1 });

    await user.click(screen.getByRole('link', { name: 'Каталог' }));
    expect(await screen.findByRole('link', { name: 'Вампиры' })).toBeInTheDocument();
  });

  it('ошибки формы приходят с сервера и стоят у полей', async () => {
    const user = userEvent.setup();
    renderApp('/admin/titles/new', { as: 'admin' });
    const form = await screen.findByRole('form', { name: 'Карточка тайтла' });
    await user.type(within(form).getByLabelText('Название'), 'Из будущего');
    await user.clear(within(form).getByLabelText('Год'));
    await user.type(within(form).getByLabelText('Год'), '1500');
    await user.click(within(form).getByRole('button', { name: 'Создать черновик' }));
    expect(await within(form).findByText('Год — от 1895 до ближайших лет')).toBeInTheDocument();
    expect(within(form).getByLabelText('Год')).toHaveAttribute('aria-invalid', 'true');
    expect(screen.getByText('Проверьте данные формы: Год — от 1895 до ближайших лет')).toBeInTheDocument();
  });

  it('публикация без описания и серий: сервер объясняет, чего не хватает', async () => {
    const user = userEvent.setup();
    renderApp('/admin/titles/new', { as: 'admin' });
    const form = await screen.findByRole('form', { name: 'Карточка тайтла' });
    await user.selectOptions(within(form).getByLabelText('Тип'), 'series');
    await user.type(within(form).getByLabelText('Название'), 'Юдекс');
    await user.click(within(form).getByRole('button', { name: 'Создать черновик' }));
    await user.click(await screen.findByRole('button', { name: 'Опубликовать' }));
    expect(
      await screen.findByText(
        'Проверьте данные формы: Перед публикацией добавьте описание (от 20 символов); Добавьте хотя бы одну серию',
      ),
    ).toBeInTheDocument();
    expect(screen.getByLabelText('Описание')).toHaveAttribute('aria-invalid', 'true');
    expect(db.titles.find((t) => t.name === 'Юдекс')?.status).toBe('draft');
  });

  it('снимает фильм с показа с причиной; зритель его больше не видит', async () => {
    const user = userEvent.setup();
    renderApp('/admin/titles/t-moon', { as: 'admin' });
    await user.click(await screen.findByRole('button', { name: 'Снять с показа' }));
    const dialog = screen.getByRole('dialog', { name: 'Снять с показа' });
    await user.type(within(dialog).getByLabelText('Причина'), 'Истекли права');
    await user.click(within(dialog).getByRole('button', { name: 'Снять с показа' }));
    expect(await screen.findByText('Причина: Истекли права')).toBeInTheDocument();
    expect(db.titles.find((t) => t.id === 't-moon')?.status).toBe('withdrawn');
  });

  it('устаревшая ревизия: правка из другой вкладки не затирается', async () => {
    const user = userEvent.setup();
    renderApp('/admin/titles/t-metropolis', { as: 'admin' });
    const form = await screen.findByRole('form', { name: 'Карточка тайтла' });
    db.titles.find((t) => t.id === 't-metropolis')!.revision += 1; // кто-то сохранил раньше
    await user.type(within(form).getByLabelText('Описание'), ' Реставрация 2010 года.');
    await user.click(within(form).getByRole('button', { name: 'Сохранить' }));
    expect(
      await screen.findByText('Тайтл изменили в другой вкладке — обновите страницу'),
    ).toBeInTheDocument();
  });

  it('удаляет черновик', async () => {
    const user = userEvent.setup();
    renderApp('/admin/titles/t-greed', { as: 'admin' });
    await user.click(await screen.findByRole('button', { name: 'Удалить' }));
    await user.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Удалить' }));
    expect(await screen.findByRole('heading', { name: 'Каталог' })).toBeInTheDocument();
    expect(db.titles.some((t) => t.id === 't-greed')).toBe(false);
  });

  it('фильтрует тайтлы по статусу', async () => {
    const user = userEvent.setup();
    renderApp('/admin/titles', { as: 'admin' });
    const table = await screen.findByRole('table', { name: 'Тайтлы' });
    expect(within(table).getAllByRole('row')).toHaveLength(8); // шапка + 7
    await user.selectOptions(screen.getByLabelText('Статус'), 'draft');
    await waitFor(() => expect(within(screen.getByRole('table')).getAllByRole('row')).toHaveLength(2));
  });

  it('продажи: выручка и возврат с отзывом доступа', async () => {
    const user = userEvent.setup();
    renderApp('/admin/sales', { as: 'admin' });
    expect(await screen.findByRole('figure', { name: 'Выручка' })).toHaveTextContent('99');
    const table = await screen.findByRole('table', { name: 'Заказы' });
    await user.click(within(table).getByRole('button', { name: 'Вернуть' }));
    const dialog = screen.getByRole('dialog', { name: 'Возврат денег' });
    await user.type(within(dialog).getByLabelText('Причина возврата'), 'Жалоба на качество');
    await user.click(within(dialog).getByRole('button', { name: 'Вернуть деньги' }));
    expect(await screen.findByRole('figure', { name: 'Возвратов' })).toHaveTextContent('1');
    expect(db.purchases.some((p) => p.title_id === 't-general')).toBe(false);
  });
});
