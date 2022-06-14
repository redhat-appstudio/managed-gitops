DROP TABLE public.repositorycredentials (
    repositorycredentials_id character varying(48) NOT NULL,
    repo_cred_user_id character varying(48) NOT NULL,
    repo_cred_url character varying(512) NOT NULL,
    repo_cred_user character varying(256),
    repo_cred_pass character varying(1024),
    repo_cred_ssh character varying(1024),
    repo_cred_secret character varying(48) NOT NULL,
    repo_cred_engine_id character varying(48) NOT NULL,
    seq_id integer NOT NULL
);

DROP SEQUENCE public.repositorycredentials_seq_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;