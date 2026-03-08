import { useEffect, useState } from 'react';
import { api } from '../api';
import type { AccountsPageResponse } from '../types';

interface UseAccountsPageOptions {
  resultId?: string;
  page: number;
  pageSize: number;
  search?: string;
  status?: string;
  sort?: string;
  onlyFailure?: boolean;
}

interface UseAccountsPageResult {
  data: AccountsPageResponse | null;
  loading: boolean;
  error: string;
}

export function useAccountsPage(options: UseAccountsPageOptions): UseAccountsPageResult {
  const { resultId, page, pageSize, search, status, sort, onlyFailure } = options;
  const [data, setData] = useState<AccountsPageResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!resultId) {
      setData(null);
      setLoading(false);
      setError('');
      return;
    }

    let cancelled = false;
    setLoading(true);
    setError('');

    void api
      .getAccountsPage({
        resultId,
        page,
        pageSize,
        search,
        status,
        sort,
        onlyFailure,
      })
      .then((response) => {
        if (cancelled) return;
        setData(response);
      })
      .catch((pageError) => {
        if (cancelled) return;
        setData(null);
        setError(pageError instanceof Error ? pageError.message : '加载账户分页失败');
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [onlyFailure, page, pageSize, resultId, search, sort, status]);

  return { data, loading, error };
}
