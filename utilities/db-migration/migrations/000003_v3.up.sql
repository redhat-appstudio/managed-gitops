CREATE TABLE public.repositorycredentials (
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

ALTER TABLE public.repositorycredentials OWNER TO postgres;

CREATE SEQUENCE public.repositorycredentials_seq_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER TABLE public.repositorycredentials_seq_id_seq OWNER TO postgres;

ALTER SEQUENCE public.repositorycredentials_seq_id_seq OWNED BY public.repositorycredentials.seq_id;

ALTER TABLE ONLY public.repositorycredentials ALTER COLUMN seq_id SET DEFAULT nextval('public.repositorycredentials_seq_id_seq'::regclass);

ALTER TABLE ONLY public.repositorycredentials
    ADD CONSTRAINT repositorycredentials_pkey PRIMARY KEY (repositorycredentials_id);

ALTER TABLE ONLY public.repositorycredentials
    ADD CONSTRAINT fk_clusteruser_id FOREIGN KEY (repo_cred_user_id) REFERENCES public.clusteruser(clusteruser_id);

ALTER TABLE ONLY public.repositorycredentials
    ADD CONSTRAINT fk_gitopsengineinstance_id FOREIGN KEY (repo_cred_engine_id) REFERENCES public.gitopsengineinstance(gitopsengineinstance_id);


