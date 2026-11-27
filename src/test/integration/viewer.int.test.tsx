import { describe, expect, it } from 'vitest';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderApp } from '@/test/render';
import { db } from '@/test/gateway';
import { cardNumbers } from '@/test/fixtures';

describe('зритель: от витрины до просмотра', () => {
  it('гость видит каталог, фильтрует по жанру и открывает фильм', async () => {
    const user = userEvent.setup();
    renderApp('/');
    const grid = await screen.findByRole('region', { name: 'Каталог' });
    expect(within(grid).getAllByRole('article')).toHaveLength(6);
    await user.selectOptions(screen.getByLabelText('Жанр'), 'ужасы');
    await waitFor(() => expect(within(grid).getAllByRole('article')).toHaveLength(2));
    await user.click(within(grid).getByRole('link', { name: /Носферату/ }));
    expect(await screen.findByRole('heading', { name: 'Носферату. Симфония ужаса' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Войти, чтобы смотреть' })).toBeInTheDocument();
  });

  it('поиск по названию и «ничего не нашлось»', async () => {
    const user = userEvent.setup();
    renderApp('/');
    await screen.findByRole('region', { name: 'Каталог' });
    await user.type(screen.getByLabelText('Название'), 'луну{Enter}');
    await waitFor(() =>
      expect(within(screen.getByRole('region', { name: 'Каталог' })).getAllByRole('article')).toHaveLength(1),
    );
    await user.clear(screen.getByLabelText('Название'));
    await user.type(screen.getByLabelText('Название'), 'zzz{Enter}');
    expect(await screen.findByText(/Ничего не нашлось/)).toBeInTheDocument();
  });

  it('автопоиск: результат появляется без Enter, когда пользователь перестал печатать', async () => {
    const user = userEvent.setup();
    renderApp('/');
    const grid = await screen.findByRole('region', { name: 'Каталог' });
    await user.type(screen.getByLabelText('Название'), 'луну');
    // useDebounce ждёт 400 мс тишины, затем каталог перезапрашивается сам.
    await waitFor(() => expect(within(grid).getAllByRole('article')).toHaveLength(1), { timeout: 2000 });
  });

  it('«Продолжить просмотр» на главной у зрителя', async () => {
    renderApp('/', { as: 'viewer' });
    const row = await screen.findByRole('region', { name: 'Продолжить просмотр' });
    expect(within(row).getByRole('progressbar', { name: 'Просмотрено: Генерал' })).toHaveAttribute(
      'aria-valuenow',
      '38',
    );
  });

  it('покупает фильм картой, которую банк сначала отклонил', async () => {
    const user = userEvent.setup();
    renderApp('/title/t-nosferatu', { as: 'viewer' });
    await user.click(await screen.findByRole('button', { name: /Купить навсегда за 149/ }));
    let dialog = await screen.findByRole('dialog', { name: 'Оплата: Носферату. Симфония ужаса' });
    await user.click(within(dialog).getByRole('button', { name: 'Банк отклонил' }));
    expect(within(dialog).getByLabelText('Номер карты')).toHaveValue('4000 0000 0000 0002');
    await user.click(within(dialog).getByRole('button', { name: /Оплатить/ }));
    expect(await within(dialog).findByText('Оплата не прошла: Банк отклонил операцию')).toBeInTheDocument();

    await user.click(within(dialog).getByRole('button', { name: 'Попробовать другую карту' }));
    await user.type(within(dialog).getByLabelText('Номер карты'), cardNumbers.ok);
    await user.click(within(dialog).getByRole('button', { name: /Оплатить/ }));
    dialog = await screen.findByRole('dialog', { name: 'Оплата прошла' });
    await user.click(within(dialog).getByRole('button', { name: 'Готово' }));

    expect(await screen.findByRole('link', { name: 'Смотреть' })).toBeInTheDocument();
    expect(screen.getByText('Фильм куплен — он ваш навсегда.')).toBeInTheDocument();
    expect(db.orders.filter((o) => o.title_id === 't-nosferatu').map((o) => o.status)).toEqual([
      'failed',
      'completed',
    ]);
  });

  it('оформляет подписку, когда банк «потерял» ответ, и смотрит сериал', async () => {
    const user = userEvent.setup();
    renderApp('/plans', { as: 'viewer' });
    await user.click(await screen.findByRole('button', { name: 'Оформить: Месяц' }));
    const dialog = await screen.findByRole('dialog', { name: 'Оплата: подписка «Месяц»' });
    await user.type(within(dialog).getByLabelText('Номер карты'), cardNumbers.lost);
    await user.click(within(dialog).getByRole('button', { name: /Оплатить/ }));
    expect(await screen.findByRole('dialog', { name: 'Оплата прошла' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Готово' }));
    expect(await screen.findByText(/У вас подписка «Месяц» до/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Оформить: Год' })).toBeDisabled();

    await user.click(screen.getByRole('link', { name: 'Каталог' }));
    await user.click(await screen.findByRole('link', { name: 'Фантомас' }));
    const eps = await screen.findByRole('list', { name: 'Серии' });
    await user.click(within(eps).getByRole('link', { name: 'Смотреть: Жюв против Фантомаса' }));
    expect(await screen.findByRole('heading', { name: 'Серия 2. Жюв против Фантомаса' })).toBeInTheDocument();
    expect(db.views[0]).toMatchObject({ title_id: 't-fantomas', episode_id: 'e-2', via: 'subscription' });
  });

  it('без прав плеер предлагает подписку', async () => {
    renderApp('/watch/t-metropolis', { as: 'viewer' });
    expect(await screen.findByText('Оформите подписку или купите фильм, чтобы смотреть')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Выбрать подписку' })).toBeInTheDocument();
  });

  it('купленный фильм продолжается с места остановки', async () => {
    renderApp('/title/t-general', { as: 'viewer' });
    expect(await screen.findByText('Вы остановились на 30:00 из 1:18:00')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('link', { name: 'Продолжить' }));
    expect(await screen.findByText('Продолжаем с 30:00')).toBeInTheDocument();
    expect(screen.getByLabelText('Плеер')).toHaveAttribute('src', '/api/playback/stream/tok-t-general');
  });

  it('«Смотреть позже»: добавить на карточке и убрать в списке', async () => {
    const user = userEvent.setup();
    renderApp('/title/t-moon', { as: 'viewer' });
    const btn = await screen.findByRole('button', { name: '☆ Смотреть позже' });
    await user.click(btn);
    expect(await screen.findByRole('button', { name: '★ В списке' })).toHaveAttribute('aria-pressed', 'true');
    await user.click(screen.getByRole('link', { name: 'Смотреть позже' }));
    expect(await screen.findByRole('article', { name: 'Путешествие на Луну' })).toBeInTheDocument();
    const nosf = screen.getByRole('article', { name: 'Носферату. Симфония ужаса' });
    await user.click(within(nosf).getByRole('button', { name: 'Убрать' }));
    await waitFor(() =>
      expect(screen.queryByRole('article', { name: 'Носферату. Симфония ужаса' })).not.toBeInTheDocument(),
    );
  });

  it('18+: сначала дата рождения в профиле', async () => {
    const user = userEvent.setup();
    db.subscriptions.push({
      id: 's-max',
      user_id: 'u-max',
      plan_id: 'month',
      plan_name: 'Месяц',
      started_at: '2026-01-01T00:00:00Z',
      current_period_end: '2099-01-01T00:00:00Z',
      cancel_at_period_end: false,
    });
    renderApp('/watch/t-haxan', { as: 'newbie' });
    expect(await screen.findByText('Для фильмов 18+ укажите дату рождения в профиле')).toBeInTheDocument();
    await user.click(screen.getByRole('link', { name: 'Указать дату рождения' }));
    const form = await screen.findByRole('form', { name: 'Профиль' });
    await user.type(within(form).getByLabelText('Дата рождения'), '1990-03-03');
    await user.click(within(form).getByRole('button', { name: 'Сохранить' }));
    expect(await screen.findByText('Профиль сохранён')).toBeInTheDocument();
    expect(db.users.find((u) => u.id === 'u-max')?.birth_date).toBe('1990-03-03');
  });

  it('кабинет: заказы, история, автопродление и завершение сеанса на ТВ', async () => {
    const user = userEvent.setup();
    db.subscriptions.push({
      id: 's-anna',
      user_id: 'u-anna',
      plan_id: 'year',
      plan_name: 'Год',
      started_at: '2026-01-01T00:00:00Z',
      current_period_end: '2099-01-01T00:00:00Z',
      cancel_at_period_end: false,
    });
    renderApp('/cabinet', { as: 'viewer' });
    expect(await screen.findByRole('figure', { name: 'Куплено фильмов' })).toHaveTextContent('1');
    expect(screen.getByText(/Продлится автоматически/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Отключить автопродление' }));
    expect(await screen.findByText(/Продление отключено/)).toBeInTheDocument();
    expect(db.subscriptions[0]!.cancel_at_period_end).toBe(true);

    await user.click(screen.getByRole('tab', { name: 'Заказы' }));
    expect(within(screen.getByRole('table', { name: 'Заказы' })).getByText('Оплачен')).toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'История' }));
    expect(screen.getByRole('article')).toHaveTextContent('Генерал');

    await user.click(screen.getByRole('tab', { name: 'Устройства' }));
    expect(await screen.findByText('Smart TV / Tizen')).toBeInTheDocument();
    expect(screen.getByText('Это устройство')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Завершить сеанс' }));
    await waitFor(() => expect(screen.queryByText('Smart TV / Tizen')).not.toBeInTheDocument());
    expect(db.devices.some((d) => d.id === 'd-anna-tv')).toBe(false);
  });

  it('смена пароля', async () => {
    const user = userEvent.setup();
    renderApp('/cabinet/profile', { as: 'viewer' });
    const form = await screen.findByRole('form', { name: 'Смена пароля' });
    await user.type(within(form).getByLabelText('Текущий пароль'), 'wrong-one');
    await user.type(within(form).getByLabelText('Новый пароль'), 'new-password-2026');
    await user.click(within(form).getByRole('button', { name: 'Сменить пароль' }));
    expect(await within(form).findByText('Текущий пароль неверен')).toBeInTheDocument();
    await user.clear(within(form).getByLabelText('Текущий пароль'));
    await user.type(within(form).getByLabelText('Текущий пароль'), 'password123');
    await user.click(within(form).getByRole('button', { name: 'Сменить пароль' }));
    expect(await within(form).findByText(/Пароль изменён/)).toBeInTheDocument();
    expect(db.users.find((u) => u.id === 'u-anna')?.password).toBe('new-password-2026');
  });
});
